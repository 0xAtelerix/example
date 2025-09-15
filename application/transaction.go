package application

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"

	"github.com/0xAtelerix/sdk/gosdk/apptypes"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/kv"
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

func (e *Transaction[R]) Unmarshal(b []byte) error {
	return json.Unmarshal(b, e)
}

func (e Transaction[R]) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e Transaction[R]) Hash() [32]byte {
	var h [32]byte
	copy(h[:], e.TxHash)

	return h
}

type Sharding struct {
	pool map[string]*Worker // Token
	mu   sync.RWMutex
}

func NewSharding(db kv.RwDB, tokens ...string) *Sharding {
	s := &Sharding{
		pool: make(map[string]*Worker),
	}

	s.pool = make(map[string]*Worker, len(tokens))

	startChs := make([]chan struct{}, 0, len(tokens))

	for _, token := range tokens {
		w, isNew := s.addToken(token)

		if isNew {
			startCh := make(chan struct{})
			startChs = append(startChs, startCh)

			s.pool[token] = w

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

	if w, ok := s.pool[token]; ok {
		s.mu.RUnlock()

		return w, false
	}

	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.addToken(token)
}

func (s *Sharding) Task(r req, db kv.RwDB) {
	w, isNew := s.AddToken(r.token)
	if isNew {
		go w.Run(db)
	}

	w.reqCh <- r
}

func (s *Sharding) Close() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, worker := range s.pool {
		close(worker.reqCh)
	}
}

func (s *Sharding) addToken(token string) (*Worker, bool) {
	if w, ok := s.pool[token]; ok {
		return w, false
	}

	reqCh := make(chan req, 100000)
	resCh := make(chan res, 100000)
	w := &Worker{
		reqCh: reqCh,
		resCh: resCh,
	}

	s.pool[token] = w

	return w, true
}

type req struct {
	token       string
	senderKey   []byte
	receiverKey []byte
	value       uint64
	roTx        kv.Tx
}

type res struct {
	senderValue   *uint256.Int
	senderKey     []byte
	receiverValue *uint256.Int
	receiverKey   []byte
	err           error
}

type WorkerManager struct {
	reqCh  chan req
	worker *Worker
}

type Worker struct {
	reqCh chan req
	resCh chan res
}

func (w *Worker) Run(db kv.RwDB, startCh ...chan struct{}) {
	var r req

	if len(startCh) > 0 {
		startCh[0] <- struct{}{}
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	roTx, err := db.BeginRo(context.Background())
	if err != nil {
		w.resCh <- res{err: err}

		return
	}

	defer roTx.Rollback()

	for r = range w.reqCh {
		senderBalanceData, err := roTx.GetOne(accountsBucket, r.senderKey)
		if err != nil {
			w.resCh <- res{err: err}

			continue
		}

		if len(senderBalanceData) == 0 {
			w.resCh <- res{
				err: fmt.Errorf(
					"%w: sender %s, token %s, actual balance %d, value %d",
					ErrNotEnoughBalance,
					r.senderKey,
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
			w.resCh <- res{
				err: fmt.Errorf(
					"%w: sender %s, token %s, actual balance %d, value %d",
					ErrNotEnoughBalance,
					r.senderKey,
					r.token,
					senderBalance,
					r.value,
				),
			}

			continue
		}

		var receiverBalanceData []byte

		receiverBalanceData, err = roTx.GetOne(accountsBucket, r.receiverKey)
		if err != nil {
			w.resCh <- res{err: err}

			continue
		}

		receiverBalance := &uint256.Int{}
		receiverBalance.SetBytes(receiverBalanceData)

		amount := uint256.NewInt(r.value)

		// add receiver's balance
		// reduce sender's balance
		receiverBalance.Add(receiverBalance, amount)
		senderBalance.Sub(senderBalance, amount)

		w.resCh <- res{
			senderValue:   senderBalance,
			senderKey:     r.senderKey,
			receiverValue: receiverBalance,
			receiverKey:   r.receiverKey,
		}
	}

	close(w.resCh)
}

func (e Transaction[R]) Process(
	dbTx kv.RwTx,
) (res R, txs []apptypes.ExternalTransaction, err error) {
	// get sender's balance
	var senderBalanceData []byte

	senderTokenKey := AccountKey(e.Sender, e.Token)

	senderBalanceData, err = dbTx.GetOne(accountsBucket, senderTokenKey)
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

	receiverTokenKey := AccountKey(e.Receiver, e.Token)

	receiverBalanceData, err = dbTx.GetOne(accountsBucket, receiverTokenKey)
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

	err = dbTx.Put(accountsBucket, senderTokenKey, senderBalance.Bytes())
	if err != nil {
		return R{}, nil, fmt.Errorf("can't store sender's balance %w", err)
	}

	err = dbTx.Put(accountsBucket, receiverTokenKey, receiverBalance.Bytes())
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
	shardedApp.Task(req{
		token:       e.Token,
		senderKey:   GetAccountKey(e.Sender, e.Token),
		receiverKey: GetAccountKey(e.Receiver, e.Token),
		value:       e.Value,
	}, db)
}

const (
	accountsBucket = "appaccounts" // token+account -> value
)

func Tables() kv.TableCfg {
	return kv.TableCfg{
		accountsBucket: {},
	}
}

func AccountKey(sender string, token string) []byte {
	return []byte(token + sender)
}

//nolint:gochecknoglobals //only for sharding testing
var accs = make(map[string]map[string][]byte)

func GetAccountKey(sender string, token string) []byte {
	acc, ok := accs[sender]
	if !ok {
		acc = make(map[string][]byte)
		accs[sender] = acc
	}

	t, ok := acc[token]
	if !ok {
		t = AccountKey(sender, token)
		acc[token] = t
	}

	return t
}
