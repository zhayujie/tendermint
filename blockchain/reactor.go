package blockchain

import (
    "bytes"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "reflect"
    "sync"
    "time"

    "github.com/tendermint/go-wire"

    cmn "github.com/tendermint/tmlibs/common"
    "github.com/tendermint/tmlibs/log"

    "github.com/tendermint/tendermint/p2p"
    sm "github.com/tendermint/tendermint/state"
    "github.com/tendermint/tendermint/types"
)

const (
	// BlockchainChannel is a channel for blocks and status updates (`BlockStore` height)
	BlockchainChannel = byte(0x40)

	defaultChannelCapacity = 1000
	trySyncIntervalMS      = 50
	// stop syncing when last block's time is
	// within this much of the system time.
	// stopSyncingDurationMinutes = 10

	// ask for best height every 10s
	statusUpdateIntervalSeconds = 10
	// check if we should switch to consensus reactor
	switchToConsensusIntervalSeconds = 1
)

type consensusReactor interface {
	// for when we switch from blockchain reactor and fast sync to
	// the consensus machine
	SwitchToConsensus(sm.State, int)
}

// BlockchainReactor handles long-term catchup syncing.
type BlockchainReactor struct {
	p2p.BaseReactor

	mtx    sync.Mutex
	params types.ConsensusParams

	// immutable
	initialState sm.State

	blockExec *sm.BlockExecutor
	store     *BlockStore
	pool      *BlockPool
	fastSync  bool

	requestsCh <-chan BlockRequest
	timeoutsCh <-chan p2p.ID
}

// NewBlockchainReactor returns new reactor instance.
func NewBlockchainReactor(state sm.State, blockExec *sm.BlockExecutor, store *BlockStore,
	fastSync bool) *BlockchainReactor {

	if state.LastBlockHeight != store.Height() {
		cmn.PanicSanity(cmn.Fmt("state (%v) and store (%v) height mismatch", state.LastBlockHeight,
			store.Height()))
	}

	requestsCh := make(chan BlockRequest, defaultChannelCapacity)
	timeoutsCh := make(chan p2p.ID, defaultChannelCapacity)
	pool := NewBlockPool(
		store.Height()+1,
		requestsCh,
		timeoutsCh,
	)
	bcR := &BlockchainReactor{
		params:       state.ConsensusParams,
		initialState: state,
		blockExec:    blockExec,
		store:        store,
		pool:         pool,
		fastSync:     fastSync,
		requestsCh:   requestsCh,
		timeoutsCh:   timeoutsCh,
	}
	bcR.BaseReactor = *p2p.NewBaseReactor("BlockchainReactor", bcR)
	return bcR
}

// SetLogger implements cmn.Service by setting the logger on reactor and pool.
func (bcR *BlockchainReactor) SetLogger(l log.Logger) {
	bcR.BaseService.Logger = l
	bcR.pool.Logger = l
}

// OnStart implements cmn.Service.
func (bcR *BlockchainReactor) OnStart() error {
	if err := bcR.BaseReactor.OnStart(); err != nil {
		return err
	}
	if bcR.fastSync {
		err := bcR.pool.Start()
		if err != nil {
			return err
		}
		go bcR.poolRoutine()
	}
	return nil
}

// OnStop implements cmn.Service.
func (bcR *BlockchainReactor) OnStop() {
	bcR.BaseReactor.OnStop()
	bcR.pool.Stop()
}

// GetChannels implements Reactor
func (bcR *BlockchainReactor) GetChannels() []*p2p.ChannelDescriptor {
	return []*p2p.ChannelDescriptor{
		{
			ID:                BlockchainChannel,
			Priority:          10,
			SendQueueCapacity: 1000,
		},
	}
}

// AddPeer implements Reactor by sending our state to peer.
func (bcR *BlockchainReactor) AddPeer(peer p2p.Peer) {
	if !peer.Send(BlockchainChannel,
		struct{ BlockchainMessage }{&bcStatusResponseMessage{bcR.store.Height()}}) {
		// doing nothing, will try later in `poolRoutine`
	}
	// peer is added to the pool once we receive the first
	// bcStatusResponseMessage from the peer and call pool.SetPeerHeight
}

// RemovePeer implements Reactor by removing peer from the pool.
func (bcR *BlockchainReactor) RemovePeer(peer p2p.Peer, reason interface{}) {
	bcR.pool.RemovePeer(peer.ID())
}

// respondToPeer loads a block and sends it to the requesting peer,
// if we have it. Otherwise, we'll respond saying we don't have it.
// According to the Tendermint spec, if all nodes are honest,
// no node should be requesting for a block that's non-existent.
func (bcR *BlockchainReactor) respondToPeer(msg *bcBlockRequestMessage,
	src p2p.Peer) (queued bool) {

	block := bcR.store.LoadBlock(msg.Height)
	if block != nil {
		msg := &bcBlockResponseMessage{Block: block}
		return src.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg})
	}

	bcR.Logger.Info("Peer asking for a block we don't have", "src", src, "height", msg.Height)

	return src.TrySend(BlockchainChannel, struct{ BlockchainMessage }{
		&bcNoBlockResponseMessage{Height: msg.Height},
	})
}

// Receive implements Reactor by handling 4 types of messages (look below).
func (bcR *BlockchainReactor) Receive(chID byte, src p2p.Peer, msgBytes []byte) {
	_, msg, err := DecodeMessage(msgBytes, bcR.maxMsgSize())
	if err != nil {
		bcR.Logger.Error("Error decoding message", "err", err)
		return
	}

	bcR.Logger.Debug("Receive", "src", src, "chID", chID, "msg", msg)

	switch msg := msg.(type) {
	case *bcBlockRequestMessage:
		if queued := bcR.respondToPeer(msg, src); !queued {
			// Unfortunately not queued since the queue is full.
		}
	case *bcBlockResponseMessage:
		// Got a block.
		bcR.pool.AddBlock(src.ID(), msg.Block, len(msgBytes))
        //bcR.Logger.Error("blockResponse: ", "height", msg.Block.Height, "len", len(msg.Block.Txs))
		if sm.IsNewPeer && int(msg.Block.Height) % sm.SNAPSHOT_INTERVAL == 1 {
		    block := msg.Block
            //bcR.Logger.Error("初始快照的下一个区块为", "tx:", json.Marshal(msg.Block.Txs))
            if block != nil && block.Data != nil {
                for i := 0; i < len(block.Data.Txs); i++ {
                    data := block.Data.Txs[i]
                    encodeStr := hex.EncodeToString(data)
                    temptx, _ := hex.DecodeString(encodeStr)        //得到真实的tx记录
                    bcR.Logger.Error("快照的下一个区块 ", "高度:", block.Height, "交易:", string(temptx))
                    // TODO: 如果是快照交易，将自身快照的hash与交易中hash进行对比

                    //var t account.TxArg
                    //json.Unmarshal(temptx, &t)

                }
            }

            // zhayujie
            // TODO 将自身快照的hash与交易中hash进行对比
            sm.IsNewPeer = false
        }

	case *bcStatusRequestMessage:
		// Send peer our state.
		queued := src.TrySend(BlockchainChannel,
			struct{ BlockchainMessage }{&bcStatusResponseMessage{bcR.store.Height()}})
		if !queued {
			// sorry
		}
	case *bcStatusResponseMessage:
		// Got a peer status. Unverified.
		bcR.pool.SetPeerHeight(src.ID(), msg.Height)
	/*
     * @Author: zyj
     * @Desc: 增加快照请求和响应的逻辑处理
     * @Date: 19.12.07
     */
    case *bcSnapshotRequestMessage:
        bcR.Logger.Error("收到快照请求! 现在发送快照")
    	// 获取所有状态集合
    	snapshot := sm.GetSnapshot()
    	snapShopMap, _ := json.Marshal(snapshot.Content)
        src.TrySend(BlockchainChannel,
            struct{ BlockchainMessage }{&bcSnapshotResponseMessage{snapshot.Version, snapShopMap}})
    	// TODO: 生成签名，对快照sha256后，私钥签名

    	
    case *bcSnapshotResponseMessage:
		bcR.Logger.Error(cmn.Fmt("收到快照版本为 %v, 内容为%v", msg.Version, string(msg.Content)))
		//bcR.Logger.Error(cmn.Fmt("收到快照版本为 %v", msg.Version))

		// TODO: 验签，使用发送方公钥进行验签。 text文本为快照内容sha256后
    	// 处理快照
		myMap := make(map[string]string)
    	json.Unmarshal(msg.Content, &myMap)
		count := 0
		for k, v := range myMap {
		    // 快照写入
			sm.SetState([]byte(k), []byte(v))
			count++
		}
		bcR.Logger.Error(cmn.Fmt("快照写入完成, 长度为%v", count))
		// 更新当前快照
		sm.SetSnapshot(sm.Snapshot{msg.Version, myMap})

		// 更新当前区块高度为快照版本
        if msg.Version > 0 && msg.Version < 0x7fffffff {
            bcR.store.height = msg.Version     // just for test
            // 更新pool中的高度为当前高度+1
            bcR.pool.height = bcR.store.height + 1
            bcR.initialState.LastBlockHeight = bcR.store.height
            // 修改consensus模块中的区块高度
            //cs := consensus.GetConsensusState()
            //cs.Height = bcR.store.height
        }

		// 标记为新加入节点
		sm.IsNewPeer = true
    /* ---------- zyj change --------- */

    default:
		bcR.Logger.Error(cmn.Fmt("Unknown message type %v", reflect.TypeOf(msg)))
	}
}

// maxMsgSize returns the maximum allowable size of a
// message on the blockchain reactor.
func (bcR *BlockchainReactor) maxMsgSize() int {
	bcR.mtx.Lock()
	defer bcR.mtx.Unlock()
	return bcR.params.BlockSize.MaxBytes + 2
}

// updateConsensusParams updates the internal consensus params
func (bcR *BlockchainReactor) updateConsensusParams(params types.ConsensusParams) {
	bcR.mtx.Lock()
	defer bcR.mtx.Unlock()
	bcR.params = params
}

// Handle messages from the poolReactor telling the reactor what to do.
// NOTE: Don't sleep in the FOR_LOOP or otherwise slow it down!
// (Except for the SYNC_LOOP, which is the primary purpose and must be synchronous.)
func (bcR *BlockchainReactor) poolRoutine() {

	trySyncTicker := time.NewTicker(trySyncIntervalMS * time.Millisecond)
	statusUpdateTicker := time.NewTicker(statusUpdateIntervalSeconds * time.Second)
	switchToConsensusTicker := time.NewTicker(switchToConsensusIntervalSeconds * time.Second)

	blocksSynced := 0

	chainID := bcR.initialState.ChainID
	state := bcR.initialState

	lastHundred := time.Now()
	lastRate := 0.0

FOR_LOOP:
	for {
		select {
		case request := <-bcR.requestsCh: // chan BlockRequest
			peer := bcR.Switch.Peers().Get(request.PeerID)
			if peer == nil {
				continue FOR_LOOP // Peer has since been disconnected.
			}
            /*
             * @Author: zyj
             * @Desc: 增加快照请求发送逻辑，当requestsCh通道中高度为-1时触发
             * @Date: 19.11.24
             */
            if request.Height == -1 {
                bcR.Logger.Error(cmn.Fmt("现在发送同步快照请求: %v", request.PeerID))
                msg := &bcSnapshotRequestMessage{}
                peer.TrySend(BlockchainChannel, struct{ BlockchainMessage}{msg})
                continue FOR_LOOP
            }
            /* ---------- zyj change --------- */

			msg := &bcBlockRequestMessage{request.Height}
			queued := peer.TrySend(BlockchainChannel, struct{ BlockchainMessage }{msg})
			if !queued {
				// We couldn't make the request, send-queue full.
				// The pool handles timeouts, just let it go.
				continue FOR_LOOP
			}
		case peerID := <-bcR.timeoutsCh: // chan string
			// Peer timed out.
			peer := bcR.Switch.Peers().Get(peerID)
			if peer != nil {
				bcR.Switch.StopPeerForError(peer, errors.New("BlockchainReactor Timeout"))
			}
		case <-statusUpdateTicker.C:
			// ask for status updates
			go bcR.BroadcastStatusRequest() // nolint: errcheck
		case <-switchToConsensusTicker.C:
			height, numPending, lenRequesters := bcR.pool.GetStatus()
			outbound, inbound, _ := bcR.Switch.NumPeers()
			bcR.Logger.Debug("Consensus ticker", "numPending", numPending, "total", lenRequesters,
				"outbound", outbound, "inbound", inbound)
			if bcR.pool.IsCaughtUp() {
				bcR.Logger.Info("Time to switch to consensus reactor!", "height", height)
                /*
                 * @Author: zyj
                 * @Desc: 更新高度，同步至consensus模块，否则会从高度1开始共识
                 * @Date: 19.11.24
                 */
                state.LastBlockHeight = height - 1
                // ------------------------------------

				bcR.pool.Stop()
				conR := bcR.Switch.Reactor("CONSENSUS").(consensusReactor)
				conR.SwitchToConsensus(state, blocksSynced)

				break FOR_LOOP
			}
		case <-trySyncTicker.C: // chan time
			// This loop can be slow as long as it's doing syncing work.
		SYNC_LOOP:
			for i := 0; i < 10; i++ {
				// See if there are any blocks to sync.
				first, second := bcR.pool.PeekTwoBlocks()
				//bcR.Logger.Info("TrySync peeked", "first", first, "second", second)
				if first == nil || second == nil {
					// We need both to sync the first block.
					break SYNC_LOOP
				}
				firstParts := first.MakePartSet(state.ConsensusParams.BlockPartSizeBytes)
				firstPartsHeader := firstParts.Header()
				firstID := types.BlockID{first.Hash(), firstPartsHeader}
				// Finally, verify the first block using the second's commit
				// NOTE: we can probably make this more efficient, but note that calling
				// first.Hash() doesn't verify the tx contents, so MakePartSet() is
				// currently necessary.
				err := state.Validators.VerifyCommit(
					chainID, firstID, first.Height, second.LastCommit)
				if err != nil {
					bcR.Logger.Error("Error in validation", "err", err)
					peerID := bcR.pool.RedoRequest(first.Height)
					peer := bcR.Switch.Peers().Get(peerID)
					if peer != nil {
						bcR.Switch.StopPeerForError(peer, fmt.Errorf("BlockchainReactor validation error: %v", err))
					}
					break SYNC_LOOP
				} else {
					bcR.pool.PopRequest()

					// TODO: batch saves so we dont persist to disk every block
					bcR.store.SaveBlock(first, firstParts, second.LastCommit)

					// TODO: same thing for app - but we would need a way to
					// get the hash without persisting the state
					var err error
					state, err = bcR.blockExec.ApplyBlock(state, firstID, first)
					if err != nil {
						// TODO This is bad, are we zombie?
						cmn.PanicQ(cmn.Fmt("Failed to process committed block (%d:%X): %v",
							first.Height, first.Hash(), err))
					}
					blocksSynced++

					// update the consensus params
					bcR.updateConsensusParams(state.ConsensusParams)

					if blocksSynced%100 == 0 {
						lastRate = 0.9*lastRate + 0.1*(100/time.Since(lastHundred).Seconds())
						bcR.Logger.Info("Fast Sync Rate", "height", bcR.pool.height,
							"max_peer_height", bcR.pool.MaxPeerHeight(), "blocks/s", lastRate)
						lastHundred = time.Now()
					}
				}
			}
			continue FOR_LOOP
		case <-bcR.Quit():
			break FOR_LOOP
		}
	}
}

// BroadcastStatusRequest broadcasts `BlockStore` height.
func (bcR *BlockchainReactor) BroadcastStatusRequest() error {
	bcR.Switch.Broadcast(BlockchainChannel,
		struct{ BlockchainMessage }{&bcStatusRequestMessage{bcR.store.Height()}})
	return nil
}

//-----------------------------------------------------------------------------
// Messages

/*
 * @Author: zyj
 * @Desc: 增加快照消息类型常量
 * @Date: 19.11.24
 */
const (
	msgTypeBlockRequest    = byte(0x10)
	msgTypeBlockResponse   = byte(0x11)
	msgTypeNoBlockResponse = byte(0x12)
	msgTypeStatusResponse  = byte(0x20)
	msgTypeStatusRequest   = byte(0x21)
    msgTypeSnapshotRequest = byte(0x22)
    msgTypeSnapshotResponse = byte(0x30)
)

// BlockchainMessage is a generic message for this reactor.
type BlockchainMessage interface{}


/*
 * @Author: zyj
 * @Desc: 增加快照消息类型注册
 * @Date: 19.11.24
 */
var _ = wire.RegisterInterface(
	struct{ BlockchainMessage }{},
	wire.ConcreteType{&bcBlockRequestMessage{}, msgTypeBlockRequest},
	wire.ConcreteType{&bcBlockResponseMessage{}, msgTypeBlockResponse},
	wire.ConcreteType{&bcNoBlockResponseMessage{}, msgTypeNoBlockResponse},
	wire.ConcreteType{&bcStatusResponseMessage{}, msgTypeStatusResponse},
	wire.ConcreteType{&bcStatusRequestMessage{}, msgTypeStatusRequest},
    wire.ConcreteType{&bcSnapshotRequestMessage{}, msgTypeSnapshotRequest},
    wire.ConcreteType{&bcSnapshotResponseMessage{}, msgTypeSnapshotResponse},
)

// DecodeMessage decodes BlockchainMessage.
// TODO: ensure that bz is completely read.
func DecodeMessage(bz []byte, maxSize int) (msgType byte, msg BlockchainMessage, err error) {
	msgType = bz[0]
	n := int(0)
	r := bytes.NewReader(bz)
	msg = wire.ReadBinary(struct{ BlockchainMessage }{}, r, maxSize, &n, &err).(struct{ BlockchainMessage }).BlockchainMessage
	if err != nil && n != len(bz) {
		err = errors.New("DecodeMessage() had bytes left over")
	}
	return
}

//-------------------------------------

type bcBlockRequestMessage struct {
	Height int64
}

func (m *bcBlockRequestMessage) String() string {
	return cmn.Fmt("[bcBlockRequestMessage %v]", m.Height)
}

type bcNoBlockResponseMessage struct {
	Height int64
}

func (brm *bcNoBlockResponseMessage) String() string {
	return cmn.Fmt("[bcNoBlockResponseMessage %d]", brm.Height)
}

//-------------------------------------

// NOTE: keep up-to-date with maxBlockchainResponseSize
type bcBlockResponseMessage struct {
	Block *types.Block
}

func (m *bcBlockResponseMessage) String() string {
	return cmn.Fmt("[bcBlockResponseMessage %v]", m.Block.Height)
}

//-------------------------------------

type bcStatusRequestMessage struct {
	Height int64
}

func (m *bcStatusRequestMessage) String() string {
	return cmn.Fmt("[bcStatusRequestMessage %v]", m.Height)
}

//-------------------------------------

type bcStatusResponseMessage struct {
	Height int64
}

func (m *bcStatusResponseMessage) String() string {
	return cmn.Fmt("[bcStatusResponseMessage %v]", m.Height)
}


//-------------------------------------
/*
 * @Author: zyj
 * @Desc: 增加快照消息类型定义
 * @Date: 19.11.24
 */
type bcSnapshotRequestMessage struct {
}

func (m *bcSnapshotRequestMessage) String() string {
    return cmn.Fmt("[bcStatusResponseMessage %v]")
}


type bcSnapshotResponseMessage struct {
    Version int64
    Content []byte
}

func (m *bcSnapshotResponseMessage) String() string {
    return cmn.Fmt("[bcStatusResponseMessage %v]", m.Version)
}

