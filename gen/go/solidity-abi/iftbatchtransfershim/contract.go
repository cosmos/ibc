// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iftbatchtransfershim

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// IFTBatchTransferShimTransfer is an auto generated low-level Go binding around an user-defined struct.
type IFTBatchTransferShimTransfer struct {
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp uint64
}

// IFTBatchTransferShimMetaData contains all meta data concerning the IFTBatchTransferShim contract.
var IFTBatchTransferShimMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"batchIftTransfer\",\"inputs\":[{\"name\":\"ift\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"clientId\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"transfers\",\"type\":\"tuple[]\",\"internalType\":\"structIFTBatchTransferShim.Transfer[]\",\"components\":[{\"name\":\"receiver\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"timeoutTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"}]",
	Bin: "0x60808060405234601557610347908161001a8239f35b5f80fdfe60806040526004361015610011575f80fd5b5f3560e01c634931275714610024575f80fd5b3461028b5760607ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc36011261028b5760043573ffffffffffffffffffffffffffffffffffffffff811680910361028b5760243567ffffffffffffffff811161028b573660238201121561028b57806004013567ffffffffffffffff811161028b576024820191602482369201011161028b576044359067ffffffffffffffff821161028b573660238301121561028b5781600401359367ffffffffffffffff851161028b576024830192602436918760051b01011161028b575f5b85811061010857005b61011381878661028f565b8035907fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe18136030182121561028b57019081359167ffffffffffffffff831161028b57602001823603811361028b57602061016f838a8961028f565b013590604061017f848b8a61028f565b01359067ffffffffffffffff821680920361028b57853b1561028b575f926101e3926102138b9360405198899687967f711708b3000000000000000000000000000000000000000000000000000000008852608060048901528d60848901916102fc565b917ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc8784030160248801526102fc565b9160448401526064830152038183875af1801561028057610239575b60019150016100ff565b67ffffffffffffffff82116102535760019160405261022f565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52604160045260245ffd5b6040513d5f823e3d90fd5b5f80fd5b91908110156102cf5760051b810135907fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffa18136030182121561028b570190565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52603260045260245ffd5b601f82602094937fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe093818652868601375f858286010152011601019056fea164736f6c634300081c000a",
}

// IFTBatchTransferShimABI is the input ABI used to generate the binding from.
// Deprecated: Use IFTBatchTransferShimMetaData.ABI instead.
var IFTBatchTransferShimABI = IFTBatchTransferShimMetaData.ABI

// IFTBatchTransferShimBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use IFTBatchTransferShimMetaData.Bin instead.
var IFTBatchTransferShimBin = IFTBatchTransferShimMetaData.Bin

// DeployIFTBatchTransferShim deploys a new Ethereum contract, binding an instance of IFTBatchTransferShim to it.
func DeployIFTBatchTransferShim(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *IFTBatchTransferShim, error) {
	parsed, err := IFTBatchTransferShimMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(IFTBatchTransferShimBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &IFTBatchTransferShim{IFTBatchTransferShimCaller: IFTBatchTransferShimCaller{contract: contract}, IFTBatchTransferShimTransactor: IFTBatchTransferShimTransactor{contract: contract}, IFTBatchTransferShimFilterer: IFTBatchTransferShimFilterer{contract: contract}}, nil
}

// IFTBatchTransferShim is an auto generated Go binding around an Ethereum contract.
type IFTBatchTransferShim struct {
	IFTBatchTransferShimCaller     // Read-only binding to the contract
	IFTBatchTransferShimTransactor // Write-only binding to the contract
	IFTBatchTransferShimFilterer   // Log filterer for contract events
}

// IFTBatchTransferShimCaller is an auto generated read-only Go binding around an Ethereum contract.
type IFTBatchTransferShimCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IFTBatchTransferShimTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IFTBatchTransferShimTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IFTBatchTransferShimFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IFTBatchTransferShimFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IFTBatchTransferShimSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IFTBatchTransferShimSession struct {
	Contract     *IFTBatchTransferShim // Generic contract binding to set the session for
	CallOpts     bind.CallOpts         // Call options to use throughout this session
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// IFTBatchTransferShimCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IFTBatchTransferShimCallerSession struct {
	Contract *IFTBatchTransferShimCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts               // Call options to use throughout this session
}

// IFTBatchTransferShimTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IFTBatchTransferShimTransactorSession struct {
	Contract     *IFTBatchTransferShimTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts               // Transaction auth options to use throughout this session
}

// IFTBatchTransferShimRaw is an auto generated low-level Go binding around an Ethereum contract.
type IFTBatchTransferShimRaw struct {
	Contract *IFTBatchTransferShim // Generic contract binding to access the raw methods on
}

// IFTBatchTransferShimCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IFTBatchTransferShimCallerRaw struct {
	Contract *IFTBatchTransferShimCaller // Generic read-only contract binding to access the raw methods on
}

// IFTBatchTransferShimTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IFTBatchTransferShimTransactorRaw struct {
	Contract *IFTBatchTransferShimTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIFTBatchTransferShim creates a new instance of IFTBatchTransferShim, bound to a specific deployed contract.
func NewIFTBatchTransferShim(address common.Address, backend bind.ContractBackend) (*IFTBatchTransferShim, error) {
	contract, err := bindIFTBatchTransferShim(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IFTBatchTransferShim{IFTBatchTransferShimCaller: IFTBatchTransferShimCaller{contract: contract}, IFTBatchTransferShimTransactor: IFTBatchTransferShimTransactor{contract: contract}, IFTBatchTransferShimFilterer: IFTBatchTransferShimFilterer{contract: contract}}, nil
}

// NewIFTBatchTransferShimCaller creates a new read-only instance of IFTBatchTransferShim, bound to a specific deployed contract.
func NewIFTBatchTransferShimCaller(address common.Address, caller bind.ContractCaller) (*IFTBatchTransferShimCaller, error) {
	contract, err := bindIFTBatchTransferShim(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IFTBatchTransferShimCaller{contract: contract}, nil
}

// NewIFTBatchTransferShimTransactor creates a new write-only instance of IFTBatchTransferShim, bound to a specific deployed contract.
func NewIFTBatchTransferShimTransactor(address common.Address, transactor bind.ContractTransactor) (*IFTBatchTransferShimTransactor, error) {
	contract, err := bindIFTBatchTransferShim(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IFTBatchTransferShimTransactor{contract: contract}, nil
}

// NewIFTBatchTransferShimFilterer creates a new log filterer instance of IFTBatchTransferShim, bound to a specific deployed contract.
func NewIFTBatchTransferShimFilterer(address common.Address, filterer bind.ContractFilterer) (*IFTBatchTransferShimFilterer, error) {
	contract, err := bindIFTBatchTransferShim(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IFTBatchTransferShimFilterer{contract: contract}, nil
}

// bindIFTBatchTransferShim binds a generic wrapper to an already deployed contract.
func bindIFTBatchTransferShim(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IFTBatchTransferShimMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IFTBatchTransferShim *IFTBatchTransferShimRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IFTBatchTransferShim.Contract.IFTBatchTransferShimCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IFTBatchTransferShim *IFTBatchTransferShimRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.IFTBatchTransferShimTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IFTBatchTransferShim *IFTBatchTransferShimRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.IFTBatchTransferShimTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IFTBatchTransferShim *IFTBatchTransferShimCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IFTBatchTransferShim.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IFTBatchTransferShim *IFTBatchTransferShimTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IFTBatchTransferShim *IFTBatchTransferShimTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.contract.Transact(opts, method, params...)
}

// BatchIftTransfer is a paid mutator transaction binding the contract method 0x49312757.
//
// Solidity: function batchIftTransfer(address ift, string clientId, (string,uint256,uint64)[] transfers) returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimTransactor) BatchIftTransfer(opts *bind.TransactOpts, ift common.Address, clientId string, transfers []IFTBatchTransferShimTransfer) (*types.Transaction, error) {
	return _IFTBatchTransferShim.contract.Transact(opts, "batchIftTransfer", ift, clientId, transfers)
}

// BatchIftTransfer is a paid mutator transaction binding the contract method 0x49312757.
//
// Solidity: function batchIftTransfer(address ift, string clientId, (string,uint256,uint64)[] transfers) returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimSession) BatchIftTransfer(ift common.Address, clientId string, transfers []IFTBatchTransferShimTransfer) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.BatchIftTransfer(&_IFTBatchTransferShim.TransactOpts, ift, clientId, transfers)
}

// BatchIftTransfer is a paid mutator transaction binding the contract method 0x49312757.
//
// Solidity: function batchIftTransfer(address ift, string clientId, (string,uint256,uint64)[] transfers) returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimTransactorSession) BatchIftTransfer(ift common.Address, clientId string, transfers []IFTBatchTransferShimTransfer) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.BatchIftTransfer(&_IFTBatchTransferShim.TransactOpts, ift, clientId, transfers)
}
