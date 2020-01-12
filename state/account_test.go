package state

import (
    "fmt"
    dbm "github.com/tendermint/tmlibs/db"
    "github.com/tendermint/tmlibs/log"
    "testing"
)

func TestNewAccountLog(t *testing.T) {
    InitDBForTest(dbm.NewMemDB(), log.TestingLogger())
    //fmt.Println(_geneate_random_str(32))
    txStr := "{\"TxType\":\"tx\", \"Sender\":\"a\", \"Receiver\":\"b\", \"Content\":\"100\"}"
    tx := []byte(txStr)
    accountLog := NewAccountLog(tx)
    if accountLog == nil {
        t.Error("解析失败")
    } else {
        fmt.Println(accountLog)
    }
}

func TestAccountLog_Check(t *testing.T) {
    db := dbm.NewMemDB()
    InitDBForTest(db, log.TestingLogger())
    txStr := "_b_100"
    accountLog := NewAccountLog([]byte(txStr))
    res := accountLog.Check()
    fmt.Println(res)
}

func TestAccountLog_Save(t *testing.T) {
    db := dbm.NewMemDB()
    InitDBForTest(db, log.TestingLogger())
    txStr := "{\"TxType\":\"tx\", \"Sender\":\"\", \"Receiver\":\"b\", \"Content\":\"100\"}"
    accountLog := NewAccountLog([]byte(txStr))
    accountLog.Save()
    fmt.Println("b的余额为: " + getState("b", db))
}

func TestAccountLog_Check2(t *testing.T) {
    db := dbm.NewMemDB()
    InitDBForTest(db, log.TestingLogger())
    txStr1 := "_B_100"
    accountLog1 := NewAccountLog([]byte(txStr1))
    txStr2 := "_A_500"
    accountLog2 := NewAccountLog([]byte(txStr2))
    accountLog1.Save()
    accountLog2.Save()

    // 转账
    txStr3 := "A_B_200"
    accountLog3 := NewAccountLog([]byte(txStr3))
    accountLog3.Check()
}


func TestAccountLog_Save2(t *testing.T) {
    db := dbm.NewMemDB()
    InitDBForTest(db, log.TestingLogger())
    txStr1 := "{\"TxType\":\"tx\", \"Sender\":\"\", \"Receiver\":\"b\", \"Content\":\"100\"}"
    accountLog1 := NewAccountLog([]byte(txStr1))
    txStr2 := "{\"TxType\":\"tx\", \"Sender\":\"\", \"Receiver\":\"a\", \"Content\":\"500\"}"
    accountLog2 := NewAccountLog([]byte(txStr2))
    accountLog1.Save()
    accountLog2.Save()
    fmt.Println("转账前: a的余额为: " + getState("a", db) + "  b的余额为: " + getState("b", db))

    // 转账
    txStr3 := "{\"TxType\":\"tx\", \"Sender\":\"a\", \"Receiver\":\"b\", \"Content\":\"200\"}"
    accountLog3 := NewAccountLog([]byte(txStr3))
    res := accountLog3.Check()
    if !res {
        t.Error("校验不通过")
    }
    accountLog3.Save()
    fmt.Println("转账后: a的余额为: " + getState("a", db) + "  b的余额为: " + getState("b", db))
}


func TestAccountLog_Save3(t *testing.T) {
    db := dbm.NewMemDB()
    InitDBForTest(db, log.TestingLogger())
    txStr1 := "_a_100"
    accountLog1 := NewAccountLog([]byte(txStr1))
    txStr2 := "_b_500"
    accountLog2 := NewAccountLog([]byte(txStr2))
    accountLog1.Save()
    accountLog2.Save()
    fmt.Println("转账前: a的余额为: " + getState("a", db) + "  b的余额为: " + getState("b", db))

    // 转账
    txStr3 := "b_a_200"
    accountLog3 := NewAccountLog([]byte(txStr3))
    res := accountLog3.Check()
    if !res {
        t.Error("校验不通过")
    }
    accountLog3.Save()
    fmt.Println("转账后: a的余额为: " + getState("a", db) + "  b的余额为: " + getState("b", db))

    txStr4 := "a_e_200"
    accountLog4 := NewAccountLog([]byte(txStr4))
    accountLog4.Save()


    testMap := GetAllStates()
    fmt.Println(testMap)
}


func TestGenerateSnapshotFast(t *testing.T) {
    db := dbm.NewMemDB()
    InitDBForTest(db, log.TestingLogger())
    txStr1 := "_a_100"
    accountLog1 := NewAccountLog([]byte(txStr1))
    txStr2 := "_b_500"
    accountLog2 := NewAccountLog([]byte(txStr2))
    accountLog1.Save()
    accountLog2.Save()
    GenerateSnapshotFast(1)

    txStr3 := "b_a_200"
    accountLog3 := NewAccountLog([]byte(txStr3))
    accountLog3.Save()
    GenerateSnapshotFast(2)
}


func getState(account string, db dbm.DB) string {
    return string(db.Get([]byte(account)))
}

func TestDoHash(t *testing.T) {
    fmt.Println(DoHash("hello"))
}

func TestSign(t *testing.T) {
    r, s := Sign("12356", "./priv.pem")
    fmt.Println(string(r), "\n", string(s))
    fmt.Println("验签结果: ", Verify(r, s, "12356", "./pub.pem"))
}

func TestGenerateKey(t *testing.T) {
    GenerateKey("./priv.pem", "./pub.pem")
}
