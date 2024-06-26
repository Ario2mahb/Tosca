//
// Copyright (c) 2024 Fantom Foundation
//
// Use of this software is governed by the Business Source License included
// in the LICENSE file and at fantom.foundation/bsl11.
//
// Change Date: 2028-4-16
//
// On the date above, in accordance with the Business Source License, use
// of this software will be governed by the GNU Lesser General Public Licence v3
//

package geth

import (
	"errors"
	"fmt"
	"math/big"

	ct "github.com/Fantom-foundation/Tosca/go/ct/common"
	"github.com/Fantom-foundation/Tosca/go/vm"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	geth "github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func init() {
	vm.RegisterInterpreter("geth", &gethVm{})
}

type gethVm struct{}

// Defines the newest supported revision for this interpreter implementation
const newestSupportedRevision = vm.R13_Cancun

func (m *gethVm) Run(parameters vm.Parameters) (vm.Result, error) {
	if parameters.Revision > newestSupportedRevision {
		return vm.Result{}, &vm.ErrUnsupportedRevision{Revision: parameters.Revision}
	}
	evm, contract, stateDb := createGethInterpreterContext(parameters)

	output, err := evm.Interpreter().Run(contract, parameters.Input, false)

	result := vm.Result{
		Output:    output,
		GasLeft:   vm.Gas(contract.Gas),
		GasRefund: vm.Gas(stateDb.refund),
		Success:   true,
	}

	// If no error is reported, the execution ended with a STOP, RETURN, or SUICIDE.
	if err == nil {
		return result, nil
	}

	// In case of a revert the result should indicate an unsuccessful execution.
	if err == geth.ErrExecutionReverted {
		result.Success = false
		return result, nil
	}

	// In case of an issue caused by the code execution, the result should indicate
	// a failed execution but no error should be reported.
	switch {
	case errors.Is(err, geth.ErrOutOfGas),
		errors.Is(err, geth.ErrCodeStoreOutOfGas),
		errors.Is(err, geth.ErrDepth),
		errors.Is(err, geth.ErrInsufficientBalance),
		errors.Is(err, geth.ErrContractAddressCollision),
		errors.Is(err, geth.ErrExecutionReverted),
		errors.Is(err, geth.ErrMaxCodeSizeExceeded),
		errors.Is(err, geth.ErrInvalidJump),
		errors.Is(err, geth.ErrWriteProtection),
		errors.Is(err, geth.ErrReturnDataOutOfBounds),
		errors.Is(err, geth.ErrReturnDataOutOfBounds),
		errors.Is(err, geth.ErrGasUintOverflow),
		errors.Is(err, geth.ErrInvalidCode):
		return vm.Result{Success: false}, nil
	}

	if _, ok := err.(*geth.ErrStackOverflow); ok {
		return vm.Result{Success: false}, nil
	}
	if _, ok := err.(*geth.ErrStackUnderflow); ok {
		return vm.Result{Success: false}, nil
	}
	if _, ok := err.(*geth.ErrInvalidOpCode); ok {
		return vm.Result{Success: false}, nil
	}

	// In all other cases an EVM error should be reported.
	return vm.Result{}, fmt.Errorf("internal EVM error in geth: %v", err)
}

// TODO: remove once there is only one Revision definition
func vmRevisionToCt(revision vm.Revision) ct.Revision {
	switch revision {
	case vm.R07_Istanbul:
		return ct.R07_Istanbul
	case vm.R09_Berlin:
		return ct.R09_Berlin
	case vm.R10_London:
		return ct.R10_London
	case vm.R11_Paris:
		return ct.R11_Paris
	case vm.R12_Shanghai:
		return ct.R12_Shanghai
	case vm.R13_Cancun:
		return ct.R13_Cancun

	}
	panic(fmt.Sprintf("Unknown revision: %v", revision))
}

// makeChainConfig returns a chain config for the given chain ID and target revision.
// The baseline config is used as a starting point, so that any prefilled configuration from go-ethereum:params/config.go can be used.
// chainId needs to be prefilled as it may be accessed with the opcode CHAINID.
// the fork-blocks and the fork-times are set to the respective values for the given revision.
func makeChainConfig(baseline params.ChainConfig, chainId *big.Int, targetRevision ct.Revision) params.ChainConfig {
	istanbulBlock, err := ct.GetForkBlock(ct.R07_Istanbul)
	if err != nil {
		panic(fmt.Sprintf("Failed to get Istanbul fork block: %v", err))
	}
	berlinBlock, err := ct.GetForkBlock(ct.R09_Berlin)
	if err != nil {
		panic(fmt.Sprintf("Failed to get Berlin fork block: %v", err))
	}
	londonBlock, err := ct.GetForkBlock(ct.R10_London)
	if err != nil {
		panic(fmt.Sprintf("Failed to get London fork block: %v", err))
	}
	parisBlock, err := ct.GetForkBlock(ct.R11_Paris)
	if err != nil {
		panic(fmt.Sprintf("Failed to get Paris fork block: %v", err))
	}
	shanghaiTime := ct.GetForkTime(ct.R12_Shanghai)
	cancunTime := ct.GetForkTime(ct.R13_Cancun)

	chainConfig := baseline
	chainConfig.ChainID = chainId
	chainConfig.ByzantiumBlock = big.NewInt(0)
	chainConfig.IstanbulBlock = big.NewInt(0).SetUint64(istanbulBlock)
	chainConfig.BerlinBlock = big.NewInt(0).SetUint64(berlinBlock)
	chainConfig.LondonBlock = big.NewInt(0).SetUint64(londonBlock)

	if targetRevision >= ct.R11_Paris {
		chainConfig.MergeNetsplitBlock = big.NewInt(0).SetUint64(parisBlock)
	}
	if targetRevision >= ct.R12_Shanghai {
		chainConfig.ShanghaiTime = &shanghaiTime
	}
	if targetRevision >= ct.R13_Cancun {
		chainConfig.CancunTime = &cancunTime
	}

	return chainConfig
}

func currentBlock(revision ct.Revision) *big.Int {
	block, err := ct.GetForkBlock(revision)
	if err != nil {
		panic(fmt.Sprintf("Failed to get fork block for %v: %v", revision, err))
	}
	return big.NewInt(int64(block + 2))
}

func createGethInterpreterContext(parameters vm.Parameters) (*geth.EVM, *geth.Contract, *stateDbAdapter) {
	context := parameters.Context.GetTransactionContext()

	// Set hard forks for chainconfig
	chainConfig :=
		makeChainConfig(*params.AllEthashProtocolChanges,
			new(big.Int).SetBytes(context.ChainID[:]),
			vmRevisionToCt(parameters.Revision))

	// Hashing function used in the context for BLOCKHASH instruction
	getHash := func(num uint64) common.Hash {
		return common.Hash(parameters.Context.GetBlockHash(int64(num)))
	}

	var transactionContext vm.TransactionContext
	if parameters.Context != nil {
		transactionContext = parameters.Context.GetTransactionContext()
	}

	// Create empty block context based on block number
	blockCtx := geth.BlockContext{
		BlockNumber: currentBlock(vmRevisionToCt(parameters.Revision)),
		Time:        uint64(transactionContext.Timestamp),
		Difficulty:  big.NewInt(1),
		GasLimit:    uint64(transactionContext.GasLimit),
		GetHash:     getHash,
		BaseFee:     new(big.Int).SetBytes(transactionContext.BaseFee[:]),
		Transfer:    transferFunc,
		CanTransfer: canTransferFunc,
	}

	if parameters.Revision >= vm.R11_Paris {
		// Setting the random signals to geth that a post-merge (Paris) revision should be utilized.
		hash := common.BytesToHash(context.PrevRandao[:])
		blockCtx.Random = &hash
	}

	// Create empty tx context
	txCtx := geth.TxContext{
		GasPrice: new(big.Int).SetBytes(transactionContext.GasPrice[:]),
	}
	// Set interpreter variant for this VM
	config := geth.Config{}

	stateDb := &stateDbAdapter{context: parameters.Context}
	evm := geth.NewEVM(blockCtx, txCtx, stateDb, &chainConfig, config)

	evm.Origin = common.Address(context.Origin)
	evm.Context.BlockNumber = big.NewInt(context.BlockNumber)
	evm.Context.Coinbase = common.Address(context.Coinbase)
	evm.Context.Difficulty = new(big.Int).SetBytes(context.PrevRandao[:])
	evm.Context.Time = uint64(context.Timestamp)

	value := vm.ValueToUint256(parameters.Value)
	addr := geth.AccountRef(parameters.Recipient)
	contract := geth.NewContract(addr, addr, value, uint64(parameters.Gas))
	contract.CallerAddress = common.Address(parameters.Sender)
	contract.CodeAddr = &common.Address{}
	contract.Code = parameters.Code
	contract.CodeHash = crypto.Keccak256Hash(parameters.Code)
	contract.Input = parameters.Input

	return evm, contract, stateDb
}

// --- Adapter ---

// transferFunc subtracts amount from sender and adds amount to recipient using the given Db
// Now is doing nothing as this is not changing gas computation
func transferFunc(stateDB geth.StateDB, callerAddress common.Address, to common.Address, value *uint256.Int) {
	// Can be something like this:
	stateDB.SubBalance(callerAddress, value, tracing.BalanceChangeTransfer)
	stateDB.AddBalance(to, value, tracing.BalanceChangeTransfer)
}

// canTransferFunc is the signature of a transfer function
func canTransferFunc(stateDB geth.StateDB, callerAddress common.Address, value *uint256.Int) bool {
	return stateDB.GetBalance(callerAddress).Cmp(value) >= 0
}

// stateDbAdapter adapts the vm.RunContext interface for its usage as a geth.StateDB in test
// environment setups. The main purpose is to facilitate unit, integration, and conformance
// testing for the geth interpreter.
type stateDbAdapter struct {
	context         vm.RunContext
	refund          uint64
	lastBeneficiary vm.Address
}

func (s *stateDbAdapter) CreateAccount(common.Address) {
	// ignored: effect not needed in test environments
}

func (s *stateDbAdapter) CreateContract(common.Address) {
	// ignored: effect not needed in test environments
}

func (s *stateDbAdapter) SubBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason) {
	// ignored: effect not needed in test environments
}

func (s *stateDbAdapter) AddBalance(addr common.Address, balance *uint256.Int, change tracing.BalanceChangeReason) {
	// we save this address to be used as the beneficiary in a selfdestruct case.
	s.lastBeneficiary = vm.Address(addr)
}

func (s *stateDbAdapter) GetBalance(addr common.Address) *uint256.Int {
	value := s.context.GetBalance(vm.Address(addr))
	return vm.ValueToUint256(value)
}

func (s *stateDbAdapter) GetNonce(common.Address) uint64 {
	// ignored: effect not needed in test environments
	return 0
}

func (s *stateDbAdapter) SetNonce(common.Address, uint64) {
	// ignored: effect not needed in test environments
}

func (s *stateDbAdapter) GetCodeHash(addr common.Address) common.Hash {
	return common.Hash(s.context.GetCodeHash(vm.Address(addr)))
}

func (s *stateDbAdapter) GetCode(addr common.Address) []byte {
	return s.context.GetCode(vm.Address(addr))
}

func (s *stateDbAdapter) SetCode(common.Address, []byte) {
	// ignored: effect not needed in test environments
}

func (s *stateDbAdapter) GetCodeSize(addr common.Address) int {
	return s.context.GetCodeSize(vm.Address(addr))
}

func (s *stateDbAdapter) AddRefund(value uint64) {
	s.refund += value
}

func (s *stateDbAdapter) SubRefund(value uint64) {
	s.refund -= value
}

func (s *stateDbAdapter) GetRefund() uint64 {
	return s.refund
}

func (s *stateDbAdapter) GetCommittedState(addr common.Address, key common.Hash) common.Hash {
	return common.Hash(s.context.GetCommittedStorage(vm.Address(addr), vm.Key(key)))
}

func (s *stateDbAdapter) GetState(addr common.Address, key common.Hash) common.Hash {
	return common.Hash(s.context.GetStorage(vm.Address(addr), vm.Key(key)))
}

func (s *stateDbAdapter) SetState(addr common.Address, key common.Hash, value common.Hash) {
	s.context.SetStorage(vm.Address(addr), vm.Key(key), vm.Word(value))
}

func (s *stateDbAdapter) GetStorageRoot(addr common.Address) common.Hash {
	// ignored: effect not needed in test environments
	return common.Hash{}
}

func (s *stateDbAdapter) GetTransientState(addr common.Address, key common.Hash) common.Hash {
	// ignored: effect not needed in test environments (todo: implement if needed)
	panic("not implemented")
}

func (s *stateDbAdapter) SetTransientState(addr common.Address, key, value common.Hash) {
	// ignored: effect not needed in test environments (todo: implement if needed)
	panic("not implemented")
}

func (s *stateDbAdapter) SelfDestruct(addr common.Address) {
	s.context.SelfDestruct(vm.Address(addr), s.lastBeneficiary)
}

func (s *stateDbAdapter) HasSelfDestructed(addr common.Address) bool {
	return s.context.HasSelfDestructed(vm.Address(addr))
}

func (s *stateDbAdapter) Selfdestruct6780(common.Address) {
	// ignored: effect not needed in test environments
	panic("not implemented")
}

func (s *stateDbAdapter) Exist(addr common.Address) bool {
	return s.context.AccountExists(vm.Address(addr))
}

func (s *stateDbAdapter) Empty(addr common.Address) bool {
	return !s.context.AccountExists(vm.Address(addr))
}

func (s *stateDbAdapter) PrepareAccessList(sender common.Address, dest *common.Address, precompiles []common.Address, txAccesses types.AccessList) {
	// ignored: effect not needed in test environments
	panic("not implemented")
}

func (s *stateDbAdapter) AddressInAccessList(addr common.Address) bool {
	return s.context.IsAddressInAccessList(vm.Address(addr))
}

func (s *stateDbAdapter) SlotInAccessList(addr common.Address, slot common.Hash) (addressOk bool, slotOk bool) {
	return s.context.IsSlotInAccessList(vm.Address(addr), vm.Key(slot))
}

func (s *stateDbAdapter) AddAddressToAccessList(addr common.Address) {
	s.context.AccessAccount(vm.Address(addr))
}

func (s *stateDbAdapter) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	s.context.AccessStorage(vm.Address(addr), vm.Key(slot))
}

func (s *stateDbAdapter) Prepare(rules params.Rules, sender, coinbase common.Address, dest *common.Address, precompiles []common.Address, txAccesses types.AccessList) {
	// ignored: effect not needed in test environments
	panic("not implemented")
}

func (s *stateDbAdapter) RevertToSnapshot(int) {
	// ignored: effect not needed in test environments
}

func (s *stateDbAdapter) Snapshot() int {
	return 0 // not relevant in test setups
}

func (s *stateDbAdapter) AddLog(log *types.Log) {
	topics := make([]vm.Hash, 0, len(log.Topics))
	for _, cur := range log.Topics {
		topics = append(topics, vm.Hash(cur))
	}
	s.context.EmitLog(vm.Address(log.Address), topics, log.Data)
}

func (s *stateDbAdapter) AddPreimage(common.Hash, []byte) {
	panic("should not be needed in test environments")
}

func (s *stateDbAdapter) ForEachStorage(common.Address, func(common.Hash, common.Hash) bool) error {
	panic("should not be needed in test environments")
}
