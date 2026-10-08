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

// IFTBatchTransferShimRecipient is an auto generated low-level Go binding around an user-defined struct.
type IFTBatchTransferShimRecipient struct {
	Account common.Address
	Amount  *big.Int
}

// IFTBatchTransferShimTransfer is an auto generated low-level Go binding around an user-defined struct.
type IFTBatchTransferShimTransfer struct {
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp uint64
}

// IFTBatchTransferShimMetaData contains all meta data concerning the IFTBatchTransferShim contract.
var IFTBatchTransferShimMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"batchIftTransfer\",\"inputs\":[{\"name\":\"ift\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"clientId\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"transfers\",\"type\":\"tuple[]\",\"internalType\":\"structIFTBatchTransferShim.Transfer[]\",\"components\":[{\"name\":\"receiver\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"timeoutTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"batchTransfer\",\"inputs\":[{\"name\":\"erc20\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"recipients\",\"type\":\"tuple[]\",\"internalType\":\"structIFTBatchTransferShim.Recipient[]\",\"components\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"}]",
	Bin: "0x60808060405234601557610584908161001a8239f35b5f80fdfe60806040526004361015610011575f80fd5b5f3560e01c806349312757146102065763a8ad2e061461002f575f80fd5b346101ef5760407ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc3601126101ef5761006661042b565b60243567ffffffffffffffff81116101ef57366023820112156101ef5780600401359067ffffffffffffffff82116101ef576024810190602436918460061b0101116101ef5773ffffffffffffffffffffffffffffffffffffffff8316905f5b8381106100cf57005b6100da818584610567565b3573ffffffffffffffffffffffffffffffffffffffff81168091036101ef576020610106838786610567565b0135604051917fa9059cbb000000000000000000000000000000000000000000000000000000008352600483015260248201526020816044815f885af19081156101fb575f916101bd575b501561015f576001016100c6565b60646040517f08c379a000000000000000000000000000000000000000000000000000000000815260206004820152601560248201527f6572633230207472616e73666572206661696c656400000000000000000000006044820152fd5b90506020813d82116101f3575b816101d7602093836104bb565b810103126101ef575180151581036101ef5785610151565b5f80fd5b3d91506101ca565b6040513d5f823e3d90fd5b346101ef5760607ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc3601126101ef5761023d61042b565b6024359067ffffffffffffffff82116101ef57366023830112156101ef57816004013567ffffffffffffffff81116101ef57602483019260248236920101116101ef576044359067ffffffffffffffff82116101ef57366023830112156101ef5781600401359367ffffffffffffffff85116101ef576024830192602436918760051b0101116101ef579273ffffffffffffffffffffffffffffffffffffffff165f5b8581106102e957005b6102f481878661044e565b8035907fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe1813603018212156101ef57019081359167ffffffffffffffff83116101ef5760200182360381136101ef576020610350838a8961044e565b0135906040610360848b8a61044e565b01359067ffffffffffffffff82168092036101ef57853b156101ef575f926103c4926103f48b9360405198899687967f711708b3000000000000000000000000000000000000000000000000000000008852608060048901528d6084890191610529565b917ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc878403016024880152610529565b9160448401526064830152038183875af19182156101fb5760019261041b575b50016102e0565b5f610425916104bb565b87610414565b6004359073ffffffffffffffffffffffffffffffffffffffff821682036101ef57565b919081101561048e5760051b810135907fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffa1813603018212156101ef570190565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52603260045260245ffd5b90601f7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0910116810190811067ffffffffffffffff8211176104fc57604052565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52604160045260245ffd5b601f82602094937fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe093818652868601375f8582860101520116010190565b919081101561048e5760061b019056fea164736f6c634300081c000a",
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

// BatchTransfer is a paid mutator transaction binding the contract method 0xa8ad2e06.
//
// Solidity: function batchTransfer(address erc20, (address,uint256)[] recipients) returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimTransactor) BatchTransfer(opts *bind.TransactOpts, erc20 common.Address, recipients []IFTBatchTransferShimRecipient) (*types.Transaction, error) {
	return _IFTBatchTransferShim.contract.Transact(opts, "batchTransfer", erc20, recipients)
}

// BatchTransfer is a paid mutator transaction binding the contract method 0xa8ad2e06.
//
// Solidity: function batchTransfer(address erc20, (address,uint256)[] recipients) returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimSession) BatchTransfer(erc20 common.Address, recipients []IFTBatchTransferShimRecipient) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.BatchTransfer(&_IFTBatchTransferShim.TransactOpts, erc20, recipients)
}

// BatchTransfer is a paid mutator transaction binding the contract method 0xa8ad2e06.
//
// Solidity: function batchTransfer(address erc20, (address,uint256)[] recipients) returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimTransactorSession) BatchTransfer(erc20 common.Address, recipients []IFTBatchTransferShimRecipient) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.BatchTransfer(&_IFTBatchTransferShim.TransactOpts, erc20, recipients)
}
