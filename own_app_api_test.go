package example

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/davecgh/go-spew/spew"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/stretchr/testify/require"
)

func TestTxPool(t *testing.T) {
	localDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(t.TempDir()).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return txpool.Tables()
		}).
		Open()
	require.NoError(t, err)

	txPool := txpool.NewTxPool[ExampleTransaction](localDB)

	// add
	var body = []byte(`{"jsonrpc":"2.0","id":1,"method":"SendTransaction","params":{"transaction":{"sender":"alice","value":42, "hash":"deadbeef"}}}`)

	var reqAdd RPCRequest
	err = json.Unmarshal(body, &reqAdd)
	require.NoError(t, err)

	var respAdd RPCResponse
	respAdd.ID = reqAdd.ID
	respAdd.JSONRPC = "2.0"

	if reqAdd.Method != "SendTransaction" {
		t.Error("wrong method", reqAdd.Method)
	}

	var txReqAdd SendTransactionRequest[ExampleTransaction]

	if err := json.Unmarshal(reqAdd.Params, &txReqAdd); err != nil {
		respAdd.Error = fmt.Sprintf("Invalid parameters: %s, %s", err.Error(), reqAdd.Params)
	} else {
		err := txPool.AddTransaction(t.Context(), txReqAdd.Transaction)
		if err != nil {
			respAdd.Error = fmt.Sprintf("Failed to add transaction: %s, %s", err.Error(), spew.Sdump(txReqAdd.Transaction))
		} else {
			respAdd.Result = map[string]string{"message": "Transaction added"}
		}

		txHash := txReqAdd.Transaction.Hash()
		txFromGet, err := txPool.GetTransaction(t.Context(), txHash[:])

		require.Equal(t, txReqAdd.Transaction, txFromGet)
	}

	txHash := txReqAdd.Transaction.Hash()

	// get
	body = []byte(`{"jsonrpc":"2.0","id":2,"method":"GetTransactionByHash","params":{"hash":"deadbeef"}}`)

	var reqGet RPCRequest
	err = json.Unmarshal(body, &reqGet)
	require.NoError(t, err)

	var respGet RPCResponse
	respGet.ID = reqGet.ID
	respGet.JSONRPC = "2.0"

	if reqGet.Method != "GetTransactionByHash" {
		t.Error("wrong method", reqGet.Method)
	}

	var txReqGet GetTransactionByHashRequest
	var gotTx ExampleTransaction

	if err = json.Unmarshal(reqGet.Params, &txReqGet); err != nil {
		respGet.Error = fmt.Sprintf("Invalid parameters: %s, %s", err.Error(), reqGet.Params)
	} else {
		var txHashGet [32]byte
		copy(txHashGet[:], txReqGet.Hash)

		require.Equal(t, txHash[:], txHashGet[:])

		gotTx, err = txPool.GetTransaction(t.Context(), txHashGet[:])
		if err != nil {
			respGet.Error = fmt.Sprintf("Transaction not found: %s, %s, %v", err.Error(), txReqGet.Hash, txHashGet[:])
		} else {
			respGet.Result = gotTx
		}
	}

	fmt.Printf("respAdd: %s\n", spew.Sdump(respAdd))
	fmt.Printf("respGet: %s\n", spew.Sdump(respGet))

	require.Equal(t, txReqAdd.Transaction, gotTx)
}
