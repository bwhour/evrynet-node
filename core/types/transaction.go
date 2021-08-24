// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package types

import (
	"container/heap"
	"errors"
	"io"
	"math/big"
	"sync/atomic"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/hexutil"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

//go:generate gencodec -type txdata -field-override txdataMarshaling -out gen_tx_json.go

type TransactionType uint

const (
	NormalTxType TransactionType = iota
	AddProviderTxType
	RemoveProviderTxType
)

var (
	ErrInvalidSig = errors.New("invalid transaction v, r, s values")
	// ErrRedundantProviderSignature is returned if the transaction create contract or modify provider but have a provider signature.
	ErrRedundantProviderSignature = errors.New("redundant provider signature")
	// ErrOwnerRequired is returned if the transaction to create contract with a provider but not contains an owner.
	ErrOwnerRequired = errors.New("owner is required")
	// ErrEmptyProvider is returned if provider is 0
	ErrEmptyProvider = errors.New("provider is 0")
	// ErrEmptyOwner is returned if owner is 0
	ErrEmptyOwner = errors.New("owner is 0")
	// ErrInvalidExtraDataType is returned if extra data type is invalid
	ErrInvalidExtraDataType = errors.New("unsupported extra data type")
)

// CreateAccountOption contain extra parameter for Account creation
type CreateAccountOption struct {
	OwnerAddress    *common.Address
	ProviderAddress *common.Address
}

type Transaction struct {
	data txdata
	// caches
	hash atomic.Value
	size atomic.Value
	from atomic.Value
}

//ethTxData is the original ethTxData format. It is kept for backward compatibility purpose.
type ethTxData struct {
	AccountNonce uint64          `json:"nonce"    gencodec:"required"`
	Price        *big.Int        `json:"gasPrice" gencodec:"required"`
	GasLimit     uint64          `json:"gas"      gencodec:"required"`
	Recipient    *common.Address `json:"to"       rlp:"nil"` // nil means contract creation
	Amount       *big.Int        `json:"value"    gencodec:"required"`
	Payload      []byte          `json:"input"    gencodec:"required"`

	// Signature values
	V *big.Int `json:"v" gencodec:"required"`
	R *big.Int `json:"r" gencodec:"required"`
	S *big.Int `json:"s" gencodec:"required"`

	// This is only used when marshaling to JSON.
	Hash *common.Hash `json:"hash" rlp:"-"`
}

func (d ethTxData) toTxData() txdata {
	return txdata{
		AccountNonce: d.AccountNonce,
		Price:        d.Price,
		GasLimit:     d.GasLimit,
		Recipient:    d.Recipient,
		Amount:       d.Amount,
		Payload:      d.Payload,

		V:    d.V,
		S:    d.S,
		R:    d.R,
		Hash: d.Hash,
	}
}

type txdata struct {
	AccountNonce uint64          `json:"nonce"    gencodec:"required"`
	Price        *big.Int        `json:"gasPrice" gencodec:"required"`
	GasLimit     uint64          `json:"gas"      gencodec:"required"`
	Recipient    *common.Address `json:"to"       rlp:"nil"` // nil means contract creation
	Amount       *big.Int        `json:"value"    gencodec:"required"`
	Payload      []byte          `json:"input"    gencodec:"required"`

	// data to create enterprise contract
	Owner    *common.Address `json:"owner" rlp:"nil"`
	Provider *common.Address `json:"provider" rlp:"nil"`
	// data to modify providers transaction
	Extra []byte `json:"extraData" rlp:"nil"`
	// Signature values
	V *big.Int `json:"v" gencodec:"required"`
	R *big.Int `json:"r" gencodec:"required"`
	S *big.Int `json:"s" gencodec:"required"`
	//Provider Signature values
	PV *big.Int `json:"pv"       rlp:"nil"`
	PR *big.Int `json:"pr"       rlp:"nil"`
	PS *big.Int `json:"ps"       rlp:"nil"`

	// This is only used when marshaling to JSON.
	Hash *common.Hash `json:"hash" rlp:"-"`
}

func (d txdata) toEthTxData() ethTxData {
	return ethTxData{
		AccountNonce: d.AccountNonce,
		Price:        d.Price,
		GasLimit:     d.GasLimit,
		Recipient:    d.Recipient,
		Amount:       d.Amount,
		Payload:      d.Payload,
		V:            d.V,
		R:            d.R,
		S:            d.S,
	}
}

func (d txdata) isEthTxData() bool {
	if d.Owner != nil {
		return false
	}
	if d.Provider != nil {
		return false
	}
	if (d.PR != nil) || (d.PS != nil) || (d.PV != nil) {
		return false
	}

	if len(d.Extra) != 0 {
		return false
	}
	return true
}

// TransactionExtraData is info about special transaction
type TransactionExtraData struct {
	Type TransactionType
	Msg  []byte
}

// ModifyProvidersMsg is info about adding or removing provider for enterprise contract
type ModifyProvidersMsg struct {
	Provider common.Address
}

type txdataMarshaling struct {
	AccountNonce hexutil.Uint64
	Price        *hexutil.Big
	GasLimit     hexutil.Uint64
	Amount       *hexutil.Big
	Payload      hexutil.Bytes
	V            *hexutil.Big
	R            *hexutil.Big
	S            *hexutil.Big
	PV           *hexutil.Big
	PR           *hexutil.Big
	PS           *hexutil.Big
	Extra        hexutil.Bytes
}

func NewTransaction(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	return newTransaction(nonce, &to, amount, gasLimit, gasPrice, data, nil)
}

// NewContractCreation create new instance of the tx with contract creation
func NewContractCreation(nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte, opts ...CreateAccountOption) *Transaction {
	tx := newTransaction(nonce, nil, amount, gasLimit, gasPrice, data, nil)
	if len(opts) > 0 {
		tx.data.Owner = opts[0].OwnerAddress
		tx.data.Provider = opts[0].ProviderAddress
	}
	return tx
}

// NewModifyProvidersTransaction create a new transaction to add/remove provider
func NewModifyProvidersTransaction(nonce uint64, to common.Address, gasLimit uint64, gasPrice *big.Int, provider common.Address, isAdd bool) (*Transaction, error) {
	addProviderMsg := &ModifyProvidersMsg{Provider: provider}
	msg, err := rlp.EncodeToBytes(addProviderMsg)
	if err != nil {
		return nil, err
	}
	txType := AddProviderTxType
	if !isAdd {
		txType = RemoveProviderTxType
	}
	extraData := &TransactionExtraData{Type: txType, Msg: msg}
	extra, err := rlp.EncodeToBytes(extraData)
	if err != nil {
		return nil, err
	}
	return newTransaction(nonce, &to, big.NewInt(0), gasLimit, gasPrice, nil, extra), nil
}

func newTransaction(nonce uint64, to *common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte, extra []byte) *Transaction {
	if len(data) > 0 {
		data = common.CopyBytes(data)
	}
	d := txdata{
		AccountNonce: nonce,
		Recipient:    to,
		Payload:      data,
		Amount:       new(big.Int),
		GasLimit:     gasLimit,
		Price:        new(big.Int),
		V:            new(big.Int),
		R:            new(big.Int),
		S:            new(big.Int),
		Extra:        extra,
	}
	if amount != nil {
		d.Amount.Set(amount)
	}
	if gasPrice != nil {
		d.Price.Set(gasPrice)
	}

	return &Transaction{data: d}
}

// ChainId returns which chain id this transaction was signed for (if at all)
func (tx *Transaction) ChainId() *big.Int {
	return deriveChainId(tx.data.V)
}

// Protected returns whether the transaction is protected from replay protection.
func (tx *Transaction) Protected() bool {
	return isProtectedV(tx.data.V)
}

// ProviderProtected returns whether the transaction is protected from replay protection.
func (tx *Transaction) ProviderProtected() bool {
	return isProtectedV(tx.data.PV)
}

func isProtectedV(V *big.Int) bool {
	if V.BitLen() <= 8 {
		v := V.Uint64()
		return v != 27 && v != 28
	}
	// anything not 27 or 28 is considered protected
	return true
}

// EncodeRLP implements rlp.Encoder
func (tx *Transaction) EncodeRLP(w io.Writer) error {
	//if txdata doesn't involve with Evrynet fee scheme, encode it as Ethereum tx for backward compatibility
	if tx.data.isEthTxData() {
		d := tx.data.toEthTxData()
		return rlp.Encode(w, &d)
	}
	return rlp.Encode(w, &tx.data)
}

// DecodeRLP implements rlp.Decoder
func (tx *Transaction) DecodeRLP(s *rlp.Stream) error {
	var (
		err error
	)
	raw, err := s.Raw()
	lenStream := uint64(len(raw))

	if err != nil {
		return err
	}

	var dataWithProvider txdata
	err = rlp.DecodeBytes(raw, &dataWithProvider)

	if err == nil {
		tx.data = dataWithProvider
		tx.size.Store(common.StorageSize(rlp.ListSize(lenStream)))
		return nil
	}

	var dataNormal ethTxData
	err = rlp.DecodeBytes(raw, &dataNormal)
	if err == nil {
		tx.data = dataNormal.toTxData()
		// add storage for providerAddr, pv, pr, ps
		tx.size.Store(common.StorageSize(rlp.ListSize(lenStream + 32)))
		return nil
	}

	return err
}

// MarshalJSON encodes the web3 RPC transaction format.
func (tx *Transaction) MarshalJSON() ([]byte, error) {
	hash := tx.Hash()
	data := tx.data
	data.Hash = &hash
	return data.MarshalJSON()
}

// UnmarshalJSON decodes the web3 RPC transaction format.
func (tx *Transaction) UnmarshalJSON(input []byte) error {
	var dec txdata
	if err := dec.UnmarshalJSON(input); err != nil {
		return err
	}

	withSignature := dec.V.Sign() != 0 || dec.R.Sign() != 0 || dec.S.Sign() != 0
	if withSignature {
		var V byte
		if isProtectedV(dec.V) {
			chainID := deriveChainId(dec.V).Uint64()
			V = byte(dec.V.Uint64() - 35 - 2*chainID)
		} else {
			V = byte(dec.V.Uint64() - 27)
		}
		if !crypto.ValidateSignatureValues(V, dec.R, dec.S, false) {
			return ErrInvalidSig
		}
	}

	*tx = Transaction{data: dec}
	return nil
}

func (tx *Transaction) Data() []byte       { return common.CopyBytes(tx.data.Payload) }
func (tx *Transaction) ExtraData() []byte  { return common.CopyBytes(tx.data.Extra) }
func (tx *Transaction) Gas() uint64        { return tx.data.GasLimit }
func (tx *Transaction) GasPrice() *big.Int { return new(big.Int).Set(tx.data.Price) }
func (tx *Transaction) Value() *big.Int    { return new(big.Int).Set(tx.data.Amount) }
func (tx *Transaction) Nonce() uint64      { return tx.data.AccountNonce }
func (tx *Transaction) CheckNonce() bool   { return true }

// To returns the recipient address of the transaction.
// It returns nil if the transaction is a contract creation.
func (tx *Transaction) To() *common.Address {
	if tx.data.Recipient == nil {
		return nil
	}
	to := *tx.data.Recipient
	return &to
}

// Hash hashes the RLP encoding of tx.
// It uniquely identifies the transaction.
func (tx *Transaction) Hash() common.Hash {
	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := rlpHash(tx)
	tx.hash.Store(v)
	return v
}

// Size returns the true RLP encoded storage size of the transaction, either by
// encoding and returning it, or returning a previsouly cached value.
func (tx *Transaction) Size() common.StorageSize {
	if size := tx.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, &tx.data)
	tx.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

// AsMessage returns the transaction as a core.Message.
//
// AsMessage requires a signer to derive the sender.
//
// AsMessage also validates the transaction fields
//
// XXX Rename message to something less arbitrary?
func (tx *Transaction) AsMessage(s Signer) (Message, error) {
	msg := Message{
		nonce:      tx.data.AccountNonce,
		gasLimit:   tx.data.GasLimit,
		gasPrice:   new(big.Int).Set(tx.data.Price),
		to:         tx.data.Recipient,
		amount:     tx.data.Amount,
		data:       tx.data.Payload,
		owner:      tx.data.Owner,
		provider:   tx.data.Provider,
		checkNonce: true,
	}

	var err error
	msg.from, err = Sender(s, tx)
	if err != nil {
		return msg, err
	}
	provider, err := Provider(s, tx)
	if err != nil {
		return msg, err
	}

	if tx.data.Recipient == nil { //valid contract creation params
		if provider != nil {
			return msg, ErrRedundantProviderSignature
		}
		emptyAddress := common.Address{}
		if tx.Provider() != nil {
			if *tx.data.Provider == emptyAddress {
				return msg, ErrEmptyProvider
			}
			if tx.Owner() == nil {
				return msg, ErrOwnerRequired
			}
			if *tx.data.Owner == emptyAddress {
				return msg, ErrEmptyOwner
			}
		}
	}

	if provider != nil {
		msg.hasProviderSignature = true
		msg.gasPayer = *provider
	} else {
		msg.gasPayer = msg.from
	}

	if len(tx.data.Extra) != 0 {
		if provider != nil {
			return msg, ErrRedundantProviderSignature
		}
		if msg.txType, msg.extraData, err = decodeTransactionExtraData(tx.data); err != nil {
			return msg, err
		}
	} else {
		msg.txType = NormalTxType
		msg.extraData = nil
	}
	return msg, nil
}

func decodeTransactionExtraData(data txdata) (TransactionType, interface{}, error) {
	var extraData TransactionExtraData
	if err := rlp.DecodeBytes(data.Extra, &extraData); err != nil {
		return NormalTxType, nil, err
	}
	switch extraData.Type {
	case AddProviderTxType, RemoveProviderTxType:
		var providerData ModifyProvidersMsg
		if err := rlp.DecodeBytes(extraData.Msg, &providerData); err != nil {
			return NormalTxType, nil, err
		}
		return extraData.Type, providerData, nil
	default:
		return extraData.Type, nil, ErrInvalidExtraDataType
	}
}

// WithProviderSignature returns a new transaction with the given provider signature.
// This signature needs to be in the [R || S || V] format where V is 0 or 1.
func (tx *Transaction) WithProviderSignature(signer Signer, sig []byte) (*Transaction, error) {
	r, s, v, err := signer.SignatureValues(tx, sig)
	if err != nil {
		return nil, err
	}
	cpy := &Transaction{data: tx.data}
	cpy.data.PR, cpy.data.PS, cpy.data.PV = r, s, v
	return cpy, nil
}

func (tx *Transaction) RawProviderSignatureValues() (*big.Int, *big.Int, *big.Int) {
	return tx.data.PV, tx.data.PR, tx.data.PS
}

func (tx *Transaction) Owner() *common.Address {
	return tx.data.Owner
}

func (tx *Transaction) Provider() *common.Address {
	return tx.data.Provider
}

// GasPayer returns gas payer of the transaction
// gas payer should be either provider or sender
func (tx *Transaction) GasPayer(s Signer) common.Address {
	provider, err := Provider(s, tx)
	if err == nil && provider != nil {
		// provider pays gas fee
		return *provider

	}
	from, err := Sender(s, tx)
	if err == nil {
		// sender pays gas fee
		return from
	}
	return common.Address{}
}

// WithSignature returns a new transaction with the given signature.
// This signature needs to be in the [R || S || V] format where V is 0 or 1.
func (tx *Transaction) WithSignature(signer Signer, sig []byte) (*Transaction, error) {
	r, s, v, err := signer.SignatureValues(tx, sig)
	if err != nil {
		return nil, err
	}
	cpy := &Transaction{data: tx.data}
	cpy.data.R, cpy.data.S, cpy.data.V = r, s, v
	return cpy, nil
}

// Cost returns amount + gasprice * gaslimit.
func (tx *Transaction) Cost() *big.Int {
	total := new(big.Int).Mul(tx.data.Price, new(big.Int).SetUint64(tx.data.GasLimit))
	total.Add(total, tx.data.Amount)
	return total
}

// TransactionFee returns gasprice * gaslimit
func (tx *Transaction) TransactionFee() *big.Int {
	fee := new(big.Int).Mul(tx.data.Price, new(big.Int).SetUint64(tx.data.GasLimit))
	return fee
}

// RawSignatureValues returns the V, R, S signature values of the transaction.
// The return values should not be modified by the caller.
func (tx *Transaction) RawSignatureValues() (v, r, s *big.Int) {
	return tx.data.V, tx.data.R, tx.data.S
}

// SignedProvider return the provider who signed that transaction
// If the error can not be extract from tx, a nil is returned
func (tx *Transaction) SignedProvider(s Signer) *common.Address {
	provider, err := Provider(s, tx)
	if err != nil {
		return nil
	}
	return provider
}

// Transactions is a Transaction slice type for basic sorting.
type Transactions []*Transaction

// Len returns the length of s.
func (s Transactions) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s Transactions) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s Transactions) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

// TxDifference returns a new set which is the difference between a and b.
func TxDifference(a, b Transactions) Transactions {
	keep := make(Transactions, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, tx := range b {
		remove[tx.Hash()] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash()]; !ok {
			keep = append(keep, tx)
		}
	}

	return keep
}

// TxByNonce implements the sort interface to allow sorting a list of transactions
// by their nonces. This is usually only useful for sorting transactions from a
// single account, otherwise a nonce comparison doesn't make much sense.
type TxByNonce Transactions

func (s TxByNonce) Len() int           { return len(s) }
func (s TxByNonce) Less(i, j int) bool { return s[i].data.AccountNonce < s[j].data.AccountNonce }
func (s TxByNonce) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// TxByPrice implements both the sort and the heap interface, making it useful
// for all at once sorting as well as individually adding and removing elements.
type TxByPrice Transactions

func (s TxByPrice) Len() int           { return len(s) }
func (s TxByPrice) Less(i, j int) bool { return s[i].data.Price.Cmp(s[j].data.Price) > 0 }
func (s TxByPrice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s *TxByPrice) Push(x interface{}) {
	*s = append(*s, x.(*Transaction))
}

func (s *TxByPrice) Pop() interface{} {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}

// TransactionsByPriceAndNonce represents a set of transactions that can return
// transactions in a profit-maximizing sorted order, while supporting removing
// entire batches of transactions for non-executable accounts.
type TransactionsByPriceAndNonce struct {
	txs    map[common.Address]Transactions // Per account nonce-sorted list of transactions
	heads  TxByPrice                       // Next transaction for each unique account (price heap)
	signer Signer                          // Signer for the set of transactions
}

// NewTransactionsByPriceAndNonce creates a transaction set that can retrieve
// price sorted transactions in a nonce-honouring way.
//
// Note, the input map is reowned so the caller should not interact any more with
// if after providing it to the constructor.
func NewTransactionsByPriceAndNonce(signer Signer, txs map[common.Address]Transactions) *TransactionsByPriceAndNonce {
	// Initialize a price based heap with the head transactions
	heads := make(TxByPrice, 0, len(txs))
	for from, accTxs := range txs {
		heads = append(heads, accTxs[0])
		// Ensure the sender address is from the signer
		acc, _ := Sender(signer, accTxs[0])
		txs[acc] = accTxs[1:]
		if from != acc {
			delete(txs, from)
		}
	}
	heap.Init(&heads)

	// Assemble and return the transaction set
	return &TransactionsByPriceAndNonce{
		txs:    txs,
		heads:  heads,
		signer: signer,
	}
}

// Peek returns the next transaction by price.
func (t *TransactionsByPriceAndNonce) Peek() *Transaction {
	if len(t.heads) == 0 {
		return nil
	}
	return t.heads[0]
}

// Shift replaces the current best head with the next one from the same account.
func (t *TransactionsByPriceAndNonce) Shift() {
	acc, _ := Sender(t.signer, t.heads[0])
	if txs, ok := t.txs[acc]; ok && len(txs) > 0 {
		t.heads[0], t.txs[acc] = txs[0], txs[1:]
		heap.Fix(&t.heads, 0)
	} else {
		heap.Pop(&t.heads)
	}
}

// Pop removes the best transaction, *not* replacing it with the next one from
// the same account. This should be used when a transaction cannot be executed
// and hence all subsequent ones should be discarded from the same account.
func (t *TransactionsByPriceAndNonce) Pop() {
	heap.Pop(&t.heads)
}

// Message is a fully derived transaction and implements core.Message
//
// NOTE: In a future PR this will be removed.
// TODO: to see why is this removed.
type Message struct {
	to         *common.Address
	from       common.Address
	nonce      uint64
	amount     *big.Int
	gasLimit   uint64
	gasPrice   *big.Int
	data       []byte
	checkNonce bool

	// enterprise contract creation params
	owner    *common.Address
	provider *common.Address
	// enterprise contract interaction params
	gasPayer             common.Address
	hasProviderSignature bool
	// special data params
	txType    TransactionType
	extraData interface{}
}

//TODO: add new message with gas payer so user can use eth_call with enterprise contract
func NewMessage(from common.Address, to *common.Address, nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte, checkNonce bool) Message {
	return Message{
		from:                 from,
		to:                   to,
		nonce:                nonce,
		amount:               amount,
		gasLimit:             gasLimit,
		gasPrice:             gasPrice,
		data:                 data,
		checkNonce:           checkNonce,
		gasPayer:             from,
		txType:               NormalTxType,
		hasProviderSignature: false,
	}
}

func (m Message) GasPayer() common.Address   { return m.gasPayer }
func (m Message) From() common.Address       { return m.from }
func (m Message) To() *common.Address        { return m.to }
func (m Message) Owner() *common.Address     { return m.owner }
func (m Message) Provider() *common.Address  { return m.provider }
func (m Message) GasPrice() *big.Int         { return m.gasPrice }
func (m Message) Value() *big.Int            { return m.amount }
func (m Message) Gas() uint64                { return m.gasLimit }
func (m Message) Nonce() uint64              { return m.nonce }
func (m Message) Data() []byte               { return m.data }
func (m Message) CheckNonce() bool           { return m.checkNonce }
func (m Message) TxType() TransactionType    { return m.txType }
func (m Message) ExtraData() interface{}     { return m.extraData }
func (m Message) HasProviderSignature() bool { return m.hasProviderSignature }
