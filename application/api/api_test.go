package api

import (
	"testing"

	"github.com/0xAtelerix/sdk/gosdk/txpool"
	"github.com/goccy/go-json"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	mdbxlog "github.com/ledgerwatch/log/v3"
	"github.com/stretchr/testify/require"

	"github.com/0xAtelerix/example/application/transactions"
)

func TestTxPool(t *testing.T) {
	localDB, err := mdbx.NewMDBX(mdbxlog.New()).
		Path(t.TempDir()).
		WithTableCfg(func(_ kv.TableCfg) kv.TableCfg {
			return txpool.Tables()
		}).
		Open()
	require.NoError(t, err)

	txPool := txpool.NewTxPool[transactions.Transaction[transactions.Receipt], transactions.Receipt](
		localDB,
	)

	// add
	body := []byte(
		`{"jsonrpc":"2.0","id":1,"method":"SendTransaction","params":{"transaction":{"sender":"alice","value":42, "hash":"deadbeef"}}}`,
	)

	var reqAdd RPCRequest

	err = json.Unmarshal(body, &reqAdd)
	require.NoError(t, err)

	if reqAdd.Method != SendTransactionMethod {
		t.Error("wrong method", reqAdd.Method)
	}

	var txReqAdd SendTransactionRequest

	err = json.Unmarshal(reqAdd.Params, &txReqAdd)
	require.NoError(t, err)

	err = txPool.AddTransaction(t.Context(), txReqAdd.Transaction)
	require.NoError(t, err)

	txHash := txReqAdd.Transaction.Hash()

	var txFromGet transactions.Transaction[transactions.Receipt]

	txFromGet, err = txPool.GetTransaction(t.Context(), txHash[:])
	require.NoError(t, err)

	require.Equal(t, txReqAdd.Transaction, txFromGet)

	// get
	body = []byte(
		`{"jsonrpc":"2.0","id":2,"method":"GetTransactionByHash","params":{"hash":"deadbeef"}}`,
	)

	var reqGet RPCRequest

	err = json.Unmarshal(body, &reqGet)
	require.NoError(t, err)

	if reqGet.Method != GetTransactionByHashMethod {
		t.Error("wrong method", reqGet.Method)
	}

	var (
		txReqGet GetTransactionByHashRequest
		gotTx    transactions.Transaction[transactions.Receipt]
	)

	err = json.Unmarshal(reqGet.Params, &txReqGet)
	require.NoError(t, err)

	var txHashGet [32]byte
	copy(txHashGet[:], txReqGet.Hash)

	require.Equal(t, txHash[:], txHashGet[:])

	gotTx, err = txPool.GetTransaction(t.Context(), txHashGet[:])
	require.NoError(t, err)

	require.Equal(t, txReqAdd.Transaction, gotTx)
}
