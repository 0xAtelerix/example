package transactions

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"

	db2 "github.com/0xAtelerix/example/application/db"
)

// appchain implementation
// step 1:
// your transaction
//

type Transaction[R Receipt] struct {
	Sender   string `json:"sender"`
	Value    uint64 `json:"value"`
	Receiver string `json:"receiver"`
	Token    string `json:"token"`
	TxHash   string `json:"hash"`
}

func (e Transaction[R]) Hash() [32]byte {
	var h [32]byte
	copy(h[:], e.TxHash)

	return h
}

type Sharding struct {
	Pool map[string]*Worker // Token
	mu   sync.RWMutex
}

func NewSharding(db kv.RwDB, tokens ...string) *Sharding {
	s := &Sharding{
		Pool: make(map[string]*Worker),
	}

	s.Pool = make(map[string]*Worker, len(tokens))

	startChs := make([]chan struct{}, 0, len(tokens))

	for _, token := range tokens {
		w, isNew := s.addToken(token)

		if isNew {
			startCh := make(chan struct{})
			startChs = append(startChs, startCh)

			s.Pool[token] = w

			go w.Run(db, startCh)
		}
	}

	for _, ch := range startChs {
		<-ch
	}

	return s
}

func (s *Sharding) AddToken(token string) (*Worker, bool) {
	s.mu.RLock()

	if w, ok := s.Pool[token]; ok {
		s.mu.RUnlock()

		return w, false
	}

	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.addToken(token)
}

func (s *Sharding) Task(r Req, db kv.RwDB) {
	w, isNew := s.AddToken(r.token)
	if isNew {
		go w.Run(db)
	}

	w.ReqCh <- r
}

func (s *Sharding) Close() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, worker := range s.Pool {
		close(worker.ReqCh)
	}
}

func (s *Sharding) addToken(token string) (*Worker, bool) {
	if w, ok := s.Pool[token]; ok {
		return w, false
	}

	reqCh := make(chan Req, 100000)
	resCh := make(chan Res, 100000)
	w := &Worker{
		ReqCh: reqCh,
		ResCh: resCh,
	}

	s.Pool[token] = w

	return w, true
}

type Req struct {
	token       string
	SenderKey   []byte
	ReceiverKey []byte
	value       uint64
	roTx        kv.Tx
}

type Res struct {
	SenderValue   *uint256.Int
	SenderKey     []byte
	ReceiverValue *uint256.Int
	ReceiverKey   []byte
	Err           error
}

type WorkerManager struct {
	ReqCh  chan Req
	Worker *Worker
}

type Worker struct {
	ReqCh chan Req
	ResCh chan Res
}

func (w *Worker) Run(db kv.RwDB, startCh ...chan struct{}) {
	var r Req

	if len(startCh) > 0 {
		startCh[0] <- struct{}{}
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	roTx, err := db.BeginRo(context.Background())
	if err != nil {
		w.ResCh <- Res{Err: err}

		return
	}

	defer roTx.Rollback()

	for r = range w.ReqCh {
		senderBalanceData, err := roTx.GetOne(db2.AccountsBucket, r.SenderKey)
		if err != nil {
			w.ResCh <- Res{Err: err}

			continue
		}

		if len(senderBalanceData) == 0 {
			w.ResCh <- Res{
				Err: fmt.Errorf(
					"%w: sender %s, token %s, actual balance %d, value %d",
					ErrNotEnoughBalance,
					r.SenderKey,
					r.token,
					0,
					r.value,
				),
			}

			continue
		}

		senderBalance := &uint256.Int{}
		senderBalance.SetBytes(senderBalanceData)

		if senderBalance.CmpUint64(r.value) < 0 {
			w.ResCh <- Res{
				Err: fmt.Errorf(
					"%w: sender %s, token %s, actual balance %d, value %d",
					ErrNotEnoughBalance,
					r.SenderKey,
					r.token,
					senderBalance,
					r.value,
				),
			}

			continue
		}

		var receiverBalanceData []byte

		receiverBalanceData, err = roTx.GetOne(db2.AccountsBucket, r.ReceiverKey)
		if err != nil {
			w.ResCh <- Res{Err: err}

			continue
		}

		receiverBalance := &uint256.Int{}
		receiverBalance.SetBytes(receiverBalanceData)

		amount := uint256.NewInt(r.value)

		// add receiver's balance
		// reduce sender's balance
		receiverBalance.Add(receiverBalance, amount)
		senderBalance.Sub(senderBalance, amount)

		w.ResCh <- Res{
			SenderValue:   senderBalance,
			SenderKey:     r.SenderKey,
			ReceiverValue: receiverBalance,
			ReceiverKey:   r.ReceiverKey,
		}
	}

	close(w.ResCh)
}

func (e Transaction[R]) Process(
	dbTx kv.RwTx,
) (res R, txs []apptypes.ExternalTransaction, err error) {
	// get sender's balance
	var senderBalanceData []byte

	senderTokenKey := db2.AccountKey(e.Sender, e.Token)

	senderBalanceData, err = dbTx.GetOne(db2.AccountsBucket, senderTokenKey)
	if err != nil {
		return res, txs, err
	}

	if len(senderBalanceData) == 0 {
		return R{}, nil, fmt.Errorf(
			"%w: sender %s, token %s, actual balance %d, value %d",
			ErrNotEnoughBalance,
			e.Sender,
			e.Token,
			0,
			e.Value,
		)
	}

	senderBalance := &uint256.Int{}
	senderBalance.SetBytes(senderBalanceData)

	if senderBalance.CmpUint64(e.Value) < 0 {
		return R{}, nil, fmt.Errorf(
			"%w: sender %s, token %s, actual balance %d, value %d",
			ErrNotEnoughBalance,
			e.Sender,
			e.Token,
			senderBalance,
			e.Value,
		)
	}

	var receiverBalanceData []byte

	receiverTokenKey := db2.AccountKey(e.Receiver, e.Token)

	receiverBalanceData, err = dbTx.GetOne(db2.AccountsBucket, receiverTokenKey)
	if err != nil {
		return res, txs, err
	}

	receiverBalance := &uint256.Int{}
	receiverBalance.SetBytes(receiverBalanceData)

	amount := uint256.NewInt(e.Value)

	// add receiver's balance
	// reduce sender's balance
	receiverBalance.Add(receiverBalance, amount)
	senderBalance.Sub(senderBalance, amount)

	err = dbTx.Put(db2.AccountsBucket, senderTokenKey, senderBalance.Bytes())
	if err != nil {
		return R{}, nil, fmt.Errorf("can't store sender's balance %w", err)
	}

	err = dbTx.Put(db2.AccountsBucket, receiverTokenKey, receiverBalance.Bytes())
	if err != nil {
		return R{}, nil, fmt.Errorf("can't store receiver's balance %w", err)
	}

	res = R{
		Sender:          e.Sender,
		SenderBalance:   senderBalance,
		Receiver:        e.Receiver,
		ReceiverBalance: receiverBalance,
		Token:           e.Token,
	}

	return res, []apptypes.ExternalTransaction{}, nil
}

func (e *Transaction[R]) ProcessOnSharding(db kv.RwDB, shardedApp *Sharding) {
	shardedApp.Task(Req{
		token:       e.Token,
		SenderKey:   db2.GetAccountKey(e.Sender, e.Token),
		ReceiverKey: db2.GetAccountKey(e.Receiver, e.Token),
		value:       e.Value,
	}, db)
}
