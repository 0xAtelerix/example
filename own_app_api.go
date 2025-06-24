package example

import (
	"github.com/0xAtelerix/sdk/gosdk/types"

	"encoding/json"
	"io"
	"net/http"
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
type RPCServer struct {
	Pool types.TxPoolInterface[*ExampleTransaction]
}

func (s *RPCServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		var txReq struct {
			Hash        string             `json:"hash"`
			Transaction ExampleTransaction `json:"transaction"`
		}
		if err := json.Unmarshal(req.Params, &txReq); err != nil {
			resp.Error = "Invalid parameters"
		} else {
			err := s.Pool.AddTransaction(&txReq.Transaction)
			if err != nil {
				resp.Error = "Failed to add transaction"
			} else {
				resp.Result = map[string]string{"message": "Transaction added"}
			}
		}

	case "GetTransactionByHash":
		var txReq struct {
			Hash []byte `json:"hash"`
		}
		if err := json.Unmarshal(req.Params, &txReq); err != nil {
			resp.Error = "Invalid parameters"
		} else {
			tx, err := s.Pool.GetTransaction(txReq.Hash)
			if err != nil {
				resp.Error = "Transaction not found"
			} else {
				resp.Result = tx
			}
		}

	default:
		resp.Error = "Method not found"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
