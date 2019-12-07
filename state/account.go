/*
 * @Author: zyj
 * @Desc: 账户模型
 * @Date: 19.11.10
 */

package state

import (
    "crypto/md5"
    "encoding/json"
    "fmt"
    cmn "github.com/tendermint/tmlibs/common"
    dbm "github.com/tendermint/tmlibs/db"
    "github.com/tendermint/tmlibs/log"
    "math/rand"
    "os"
    "strconv"
    "strings"
    "time"
)


/*
 * 交易数据结构
 */
type AccountLog struct {
    From        string         // 支出方
    To          string         // 接收方
    Amount      int            // 金额
}

// 接受到的交易请求，仅供测试使用
type TxArg struct {
    TxType     string   `json:"txType"`
    Sender     string   `json:"sender"`
    Receiver   string   `json:"receiver"`
    Content    string   `json:"content"`
}

// 实例化交易
func NewAccountLog(tx []byte) *AccountLog {
    if db == nil || logger == nil {
        InitAccountDB(nil)
    }
    return _parseTx(tx)
}

// 快照数据结构
type Snapshot struct {
    Version     int64                   // 版本
    Content     map[string]string       // 内容
}


/*
 * 成员函数
 */

// 校验交易
func (accountLog * AccountLog) Check() bool {
    from := accountLog.From
    to := accountLog.To
    amount := accountLog.Amount
    if amount <= 0 {
        logger.Error("金额应大于0")
        return false
    }

    //balanceToStr := _getState([]byte(to))
    balanceFromStr := _getState([]byte(from))

    if len(from) != 0 && balanceFromStr == nil {
        logger.Error("支出方账户不存在")
        return false
    }
    // 测试环境暂时允许不存在
    //if len(from) != 0 && balanceToStr == nil {
    //    logger.Error("接收方账户不存在")
    //    return false
    //}

    if len(from) != 0 {
        balanceFrom := _byte2digit(balanceFromStr)
        if balanceFrom < amount {
            logger.Error("余额不足")
            return false
        }
    }
    logger.Error("交易通过验证：" +  from + " -> " + to + "  " + strconv.Itoa(amount))
    return true
}


// 更新状态
func (accountLog * AccountLog) Save() {
    logger.Error("save：")

    // 支出
    if len(accountLog.From) != 0 {
        balanceFrom := _byte2digit(_getState([]byte(accountLog.From)))
        balanceFrom -= accountLog.Amount
        SetState([]byte(accountLog.From), _digit2byte(balanceFrom))
    }
    // 收入
    var balanceTo = 0
    if len(accountLog.From) != 0 {
        balanceTo = _byte2digit(_getState([]byte(accountLog.To)))
        balanceTo += accountLog.Amount
    } else {
        balanceTo = accountLog.Amount
    }
    SetState([]byte(accountLog.To), _digit2byte(balanceTo))
    logger.Error("交易完成：")
    logger.Error("交易完成：" +  accountLog.From + " -> " + accountLog.To + "  " + strconv.Itoa(accountLog.Amount))
    logger.Error("交易完成：22222")
    //balanceA := _getState([]byte(accountLog.From))
    //balanceB := _getState([]byte(accountLog.To))
    //logger.Error(accountLog.From + "账户余额: " + string(balanceA) + ", " + accountLog.To + "账户余额: " + string(balanceB))
    logger.Error(cmn.Fmt("状态集合为: ", GetAllStates()))
}



/*
 * 静态函数和私有函数
 */

// 全局对象
var db dbm.DB
var logger log.Logger
var snapshot Snapshot
var SNAPSHOT_INTERVAL = 10


// 获取db和logger句柄
func InitAccountDB(blockExec *BlockExecutor) {
    //db = dbm.NewMemDB()
    if blockExec == nil {
        logger = log.NewTMLogger(log.NewSyncWriter(os.Stdout))
        //db = dbm.NewMemDB()
    } else {
        db = blockExec.db
        logger = blockExec.logger
    }
}

// 为单元测试提供的初始化
func InitDBForTest(tdb dbm.DB, tLog log.Logger) {
    db = tdb
    logger = tLog
}

// 查询状态
func _getState(key []byte) []byte {
    return db.Get(key)
}

// 更新状态
func SetState(key []byte, val []byte) {
    if db != nil {
        //blockExec.db.SetSync(key, val);
        db.Set(key, val)
    }
}

// 获取所有状态集合
func GetAllStates() (map[string]string) {
    n := 10000
    kvMaps := make(map[string]string)
    //iter := db.Iterator([]byte("0"), []byte("z"))
    //for iter.Valid() {
    //    key := string(iter.Key())
    //    val := string(iter.Value())
    //    kvMaps[key] = val
    //    fmt.Println(iter.Valid())
    //    iter.Next()
    //}
    // 测试使用，作为快照集合
    for i := 0; i < n; i++ {
        address := _geneateRandomStr(32)
        t := md5.Sum([]byte(address))
        md5str := fmt.Sprintf("%x", t)
        //amout := rand.Intn(100)
        kvMaps[address] = md5str
        SetState([]byte(md5str), []byte(md5str))
    }
    return kvMaps
}

// 生成快照
func GenerateSnapshot(version int64) {
    newSnapshot := Snapshot{}
    newSnapshot.Version = version
    // 快照内容，仅供测试
    newSnapshot.Content = GetAllStates()
    snapshot = newSnapshot
}

// 获取快照
func GetSnapshot() Snapshot {
    return snapshot
}

func _geneateRandomStr(l int) string {
    str := "0123456789abcdefghijklmnopqrstuvwxyz"
    bytes := []byte(str)
    result := []byte{}
    r := rand.New(rand.NewSource(time.Now().UnixNano()))
    for i := 0; i < l; i++ {
        result = append(result, bytes[r.Intn(len(bytes))])
    }
    return string(result)
}

// 解析交易
func _parseTx(tx []byte) *AccountLog{
    args := strings.Split(string(tx), "_")
    fmt.Println(args)
    if len(args) != 3 {
        logger.Error("参数个数错误")
        return nil
    }

    amount, err := strconv.Atoi(args[2])
    if err != nil {
        logger.Error("解析失败，金额应为整数")
        return nil
    }
    accountLog := new(AccountLog)
    accountLog.From = args[0]
    accountLog.To = args[1]
    accountLog.Amount = amount
    return accountLog
}

func _parseTx1(tx []byte) *AccountLog{
    txArgs := new(TxArg)
    err := json.Unmarshal(tx, txArgs)
    if err != nil {
        logger.Error("交易解析失败")
        return nil
    }
    accountLog := new(AccountLog)
    accountLog.From = txArgs.Sender
    accountLog.To = txArgs.Receiver
    amount, err := strconv.Atoi(txArgs.Content)
    if err != nil {
        logger.Error("解析失败，金额应为整数")
        return nil
    }
    accountLog.Amount = amount

    return accountLog
}

func _parseTx2(tx []byte) *AccountLog {
    args := strings.Split(string(tx), "&")
    if len(args) != 3 {
        logger.Error("参数个数错误")
        return nil
    }
    froms := strings.Split(args[0], "=")
    tos := strings.Split(args[1], "=")
    amounts := strings.Split(args[2], "=")

    if froms[0] != "from" || tos[0] != "to" || amounts[0] != "amounts" {
        logger.Error("参数名称错误")
        return nil
    }

    amount, err := strconv.Atoi(amounts[1])
    if err != nil {
        logger.Error("解析失败，金额应为整数")
        return nil
    }
    accountLog := new(AccountLog)
    accountLog.From = froms[1]
    accountLog.To = tos[1]
    accountLog.Amount = amount

    return accountLog
}

func _parseTx3(tx []byte) *AccountLog {
    var args []string
    err := json.Unmarshal(tx, args)
    if err != nil {
        logger.Error("交易格式错误")
        return nil
    }
    if len(args) != 3 {
        logger.Error("参数个数错误")
        return nil
    }

    amount, err := strconv.Atoi(args[2])
    if err != nil {
        logger.Error("解析失败，金额应为整数")
        return nil
    }
    accountLog := new(AccountLog)
    accountLog.From = args[0]
    accountLog.To = args[1]
    accountLog.Amount = amount

    return accountLog
}





// 字节数组和数字转换
func _byte2digit(digitByte []byte) int {
    digit, _ := strconv.Atoi(string(digitByte))
    return digit
}

func _digit2byte(num int) []byte {
    return []byte(strconv.Itoa(num))
}
