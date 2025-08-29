package example

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/davecgh/go-spew/spew"
)

// JSON-RPC request
type RPCRequest struct {
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	JSONRPC string          `json:"jsonrpc"`
}

// JSON-RPC response
type RPCResponse struct {
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   string      `json:"error,omitempty"`
	JSONRPC string      `json:"jsonrpc"`
}

// RPCServer - обработчик JSON-RPC
type RPCServer[T apptypes.AppTransaction] struct {
	Pool apptypes.TxPoolInterface[T]
}

// todo: add context
func (s *RPCServer[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req RPCRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	var resp RPCResponse
	resp.ID = req.ID
	resp.JSONRPC = "2.0"

	switch req.Method {
	case "SendTransaction":
		var txReq SendTransactionRequest[T]

		if err := json.Unmarshal(req.Params, &txReq); err != nil {
			resp.Error = fmt.Sprintf("Invalid parameters: %s, %s", err.Error(), req.Params)
		} else {
			err := s.Pool.AddTransaction(context.TODO(), txReq.Transaction)
			if err != nil {
				resp.Error = fmt.Sprintf("Failed to add transaction: %s, %s", err.Error(), spew.Sdump(txReq.Transaction))
			} else {
				resp.Result = map[string]string{"message": fmt.Sprintf("Transaction added: %s", txReq.Transaction.Hash())}
			}
		}

	case "GetTransactionByHash":
		var txReq GetTransactionByHashRequest

		if err := json.Unmarshal(req.Params, &txReq); err != nil {
			resp.Error = fmt.Sprintf("Invalid parameters: %s, %s", err.Error(), req.Params)
		} else {
			var txHash [32]byte
			copy(txHash[:], txReq.Hash)

			tx, err := s.Pool.GetTransaction(context.TODO(), txHash[:])
			if err != nil {
				resp.Error = fmt.Sprintf("Transaction not found: err %s, %s, %s", err.Error(), txReq.Hash[:], txHash[:])
			} else {
				resp.Result = tx
			}
		}

	case "GetTransactionStatus":
		var txStatusReq GetTransactionStatus

		if err := json.Unmarshal(req.Params, &txStatusReq); err != nil {
			resp.Error = fmt.Sprintf("Invalid parameters: %s, %s", err.Error(), req.Params)
		} else {
			var txHash [32]byte
			copy(txHash[:], txStatusReq.Hash)

			txStatus, err := s.Pool.GetTransactionStatus(context.TODO(), txHash[:])
			if err != nil {
				resp.Error = fmt.Sprintf("Transaction not found: err %s, %s, %s", err.Error(), txStatusReq.Hash[:], txHash[:])
			} else {
				resp.Result = txStatus.String()
			}
		}

	default:
		resp.Error = "Method not found"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type SendTransactionRequest[T apptypes.AppTransaction] struct {
	Transaction T `json:"transaction"`
}

type GetTransactionByHashRequest struct {
	Hash string `json:"hash"`
}

type GetTransactionStatus struct {
	Hash string `json:"hash"`
}
