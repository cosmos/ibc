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
	ABI: "[{\"type\":\"function\",\"name\":\"batchIftTransfer\",\"inputs\":[{\"name\":\"ift\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"clientId\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"transfers\",\"type\":\"tuple[]\",\"internalType\":\"structIFTBatchTransferShim.Transfer[]\",\"components\":[{\"name\":\"receiver\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"timeoutTimestamp\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"fund\",\"inputs\":[{\"name\":\"erc20\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"recipients\",\"type\":\"tuple[]\",\"internalType\":\"structIFTBatchTransferShim.Recipient[]\",\"components\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"weiEach\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"payable\"}]",
	Bin: "0x60808060405234601557610752908161001a8239f35b5f80fdfe60806040526004361015610011575f80fd5b5f3560e01c806349312757146103e05763c9bbd9bc1461002f575f80fd5b60607ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc36011261031857610061610605565b60243567ffffffffffffffff8111610318573660238201121561031857806004013567ffffffffffffffff8111610318576024820191602436918360061b01011161031857604435908082028281048214831517156103b357340361032f5773ffffffffffffffffffffffffffffffffffffffff5f9416935b8181106100e357005b6100f66100f1828487610714565b610724565b6020610103838588610714565b013573ffffffffffffffffffffffffffffffffffffffff604051927fa9059cbb00000000000000000000000000000000000000000000000000000000845216600483015260248201526020816044815f8a5af1908115610324575f916102e6575b5015610288575f8080808673ffffffffffffffffffffffffffffffffffffffff6101926100f1888a8d610714565b165af13d15610283573d67ffffffffffffffff811161025657604051906101e160207fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0601f8401160183610695565b81525f60203d92013e5b156101f8576001016100da565b60646040517f08c379a000000000000000000000000000000000000000000000000000000000815260206004820152601660248201527f6e6174697665207472616e73666572206661696c6564000000000000000000006044820152fd5b7f4e487b71000000000000000000000000000000000000000000000000000000005f52604160045260245ffd5b6101eb565b60646040517f08c379a000000000000000000000000000000000000000000000000000000000815260206004820152601560248201527f6572633230207472616e73666572206661696c656400000000000000000000006044820152fd5b90506020813d821161031c575b8161030060209383610695565b8101031261031857518015158103610318575f610164565b5f80fd5b3d91506102f3565b6040513d5f823e3d90fd5b60846040517f08c379a000000000000000000000000000000000000000000000000000000000815260206004820152602760248201527f6d73672e76616c7565206d75737420626520776569456163682070657220726560448201527f63697069656e74000000000000000000000000000000000000000000000000006064820152fd5b7f4e487b71000000000000000000000000000000000000000000000000000000005f52601160045260245ffd5b346103185760607ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc36011261031857610417610605565b6024359067ffffffffffffffff8211610318573660238301121561031857816004013567ffffffffffffffff81116103185760248301926024823692010111610318576044359067ffffffffffffffff821161031857366023830112156103185781600401359367ffffffffffffffff8511610318576024830192602436918760051b010111610318579273ffffffffffffffffffffffffffffffffffffffff165f5b8581106104c357005b6104ce818786610628565b8035907fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe18136030182121561031857019081359167ffffffffffffffff831161031857602001823603811361031857602061052a838a89610628565b013590604061053a848b8a610628565b01359067ffffffffffffffff821680920361031857853b15610318575f9261059e926105ce8b9360405198899687967f711708b3000000000000000000000000000000000000000000000000000000008852608060048901528d60848901916106d6565b917ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc8784030160248801526106d6565b9160448401526064830152038183875af1918215610324576001926105f5575b50016104ba565b5f6105ff91610695565b876105ee565b6004359073ffffffffffffffffffffffffffffffffffffffff8216820361031857565b91908110156106685760051b810135907fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffa181360301821215610318570190565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52603260045260245ffd5b90601f7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0910116810190811067ffffffffffffffff82111761025657604052565b601f82602094937fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe093818652868601375f8582860101520116010190565b91908110156106685760061b0190565b3573ffffffffffffffffffffffffffffffffffffffff81168103610318579056fea164736f6c634300081c000a",
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

// Fund is a paid mutator transaction binding the contract method 0xc9bbd9bc.
//
// Solidity: function fund(address erc20, (address,uint256)[] recipients, uint256 weiEach) payable returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimTransactor) Fund(opts *bind.TransactOpts, erc20 common.Address, recipients []IFTBatchTransferShimRecipient, weiEach *big.Int) (*types.Transaction, error) {
	return _IFTBatchTransferShim.contract.Transact(opts, "fund", erc20, recipients, weiEach)
}

// Fund is a paid mutator transaction binding the contract method 0xc9bbd9bc.
//
// Solidity: function fund(address erc20, (address,uint256)[] recipients, uint256 weiEach) payable returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimSession) Fund(erc20 common.Address, recipients []IFTBatchTransferShimRecipient, weiEach *big.Int) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.Fund(&_IFTBatchTransferShim.TransactOpts, erc20, recipients, weiEach)
}

// Fund is a paid mutator transaction binding the contract method 0xc9bbd9bc.
//
// Solidity: function fund(address erc20, (address,uint256)[] recipients, uint256 weiEach) payable returns()
func (_IFTBatchTransferShim *IFTBatchTransferShimTransactorSession) Fund(erc20 common.Address, recipients []IFTBatchTransferShimRecipient, weiEach *big.Int) (*types.Transaction, error) {
	return _IFTBatchTransferShim.Contract.Fund(&_IFTBatchTransferShim.TransactOpts, erc20, recipients, weiEach)
}
