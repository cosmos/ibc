// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package dummylightclient

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

// IICS02ClientMsgsHeight is an auto generated low-level Go binding around an user-defined struct.
type IICS02ClientMsgsHeight struct {
	RevisionNumber uint64
	RevisionHeight uint64
}

// ILightClientMsgsMsgVerifyMembership is an auto generated low-level Go binding around an user-defined struct.
type ILightClientMsgsMsgVerifyMembership struct {
	Proof       []byte
	ProofHeight IICS02ClientMsgsHeight
	Path        [][]byte
	Value       []byte
}

// ILightClientMsgsMsgVerifyNonMembership is an auto generated low-level Go binding around an user-defined struct.
type ILightClientMsgsMsgVerifyNonMembership struct {
	Proof       []byte
	ProofHeight IICS02ClientMsgsHeight
	Path        [][]byte
}

// DummyLightClientMetaData contains all meta data concerning the DummyLightClient contract.
var DummyLightClientMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"updateResult_\",\"type\":\"uint8\",\"internalType\":\"enumILightClientMsgs.UpdateResult\"},{\"name\":\"membershipResult_\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"membershipShouldFail_\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getClientState\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"latestUpdateMsg\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"membershipResult\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"membershipShouldFail\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"misbehaviour\",\"inputs\":[{\"name\":\"misbehaviourMsg\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setMembershipResult\",\"inputs\":[{\"name\":\"membershipResult_\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"shouldFail\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setUpdateResult\",\"inputs\":[{\"name\":\"updateResult_\",\"type\":\"uint8\",\"internalType\":\"enumILightClientMsgs.UpdateResult\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"updateClient\",\"inputs\":[{\"name\":\"updateMsg\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"enumILightClientMsgs.UpdateResult\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"updateResult\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"enumILightClientMsgs.UpdateResult\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"verifyMembership\",\"inputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structILightClientMsgs.MsgVerifyMembership\",\"components\":[{\"name\":\"proof\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"proofHeight\",\"type\":\"tuple\",\"internalType\":\"structIICS02ClientMsgs.Height\",\"components\":[{\"name\":\"revisionNumber\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"revisionHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"name\":\"path\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"},{\"name\":\"value\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"verifyNonMembership\",\"inputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structILightClientMsgs.MsgVerifyNonMembership\",\"components\":[{\"name\":\"proof\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"proofHeight\",\"type\":\"tuple\",\"internalType\":\"structIICS02ClientMsgs.Height\",\"components\":[{\"name\":\"revisionNumber\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"revisionHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"name\":\"path\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"MembershipShouldFail\",\"inputs\":[]}]",
	Bin: "0x60803460ab57601f61087938819003918201601f19168301916001600160401b0383118484101760af5780849260609460405283398101031260ab57805190600382101560ab576020810151906001600160401b038216820360ab576040015180151580910360ab5760ff69ff0000000000000000005f549260481b1693169060018060501b0319161790610100600160481b039060081b1617175f556040516107b590816100c48239f35b5f80fd5b634e487b7160e01b5f52604160045260245ffdfe60806040526004361015610011575f80fd5b5f3560e01c80630bece35614610488578063155be2af1461045f578063374abcc91461043b5780634d6d9ffb146103e5578063682ed5f01461036757806380ebb08e14610343578063c20fd8b4146102f7578063d619aedd14610279578063ddba653714610269578063e6fbdbf0146101085763ef913a4b14610092575f80fd5b34610104575f60031936011261010457604051602081019080821067ffffffffffffffff8311176100d7576100d3916040525f815260405191829182610760565b0390f35b7f4e487b71000000000000000000000000000000000000000000000000000000005f52604160045260245ffd5b5f80fd5b34610104575f60031936011261010457604051805f6001546101298161070f565b8084529060018116908115610209575060011461018f575b5003601f017fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe01681019067ffffffffffffffff8211818310176100d757604082905281906100d39082610760565b60015f90815291507fb10e2d527612073b26eecdfd717e6a320cf44b4afac2b0732d9fcbe2b7fa0cf65b8183106101ed57505081016020017fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0610141565b60209193508060019154838588010152019101909183926101b9565b7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff001660208581019190915291151560051b840190910191507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe09050610141565b34610104576102773661067e565b005b346101045760406003193601126101045760043567ffffffffffffffff8116810361010457602435801515809103610104577fffffffffffffffffffffffffffffffffffffffffffff000000000000000000ff68ffffffffffffffff0069ff0000000000000000005f549360481b169360081b16911617175f555f80f35b346101045760206003193601126101045760043560038110156101045760ff7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff005f54169116175f555f80f35b34610104575f600319360112610104576100d360ff5f5416604051918291826106cf565b346101045760206003193601126101045760043567ffffffffffffffff81116101045760031960a09136030112610104575f5460ff8160481c166103bd5760209067ffffffffffffffff6040519160081c168152f35b7f23909e8e000000000000000000000000000000000000000000000000000000005f5260045ffd5b346101045760206003193601126101045760043567ffffffffffffffff81116101045760031960809136030112610104575f5460ff8160481c166103bd5760209067ffffffffffffffff6040519160081c168152f35b34610104575f60031936011261010457602060ff5f5460481c166040519015158152f35b34610104575f60031936011261010457602067ffffffffffffffff5f5460081c16604051908152f35b34610104576104963661067e565b67ffffffffffffffff81116100d7576104b060015461070f565b601f81116105dd575b505f601f82116001146105215781925f92610516575b50507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff8260011b9260031b1c1916176001555b6100d360ff5f5416604051918291826106cf565b0135905082806104cf565b7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe082169260015f527fb10e2d527612073b26eecdfd717e6a320cf44b4afac2b0732d9fcbe2b7fa0cf6915f5b8581106105c55750836001951061058d575b505050811b01600155610502565b7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff60f88560031b161c1991013516905582808061057f565b9092602060018192868601358155019401910161056d565b60015f52601f820160051c7fb10e2d527612073b26eecdfd717e6a320cf44b4afac2b0732d9fcbe2b7fa0cf6019060208310610656575b601f0160051c7fb10e2d527612073b26eecdfd717e6a320cf44b4afac2b0732d9fcbe2b7fa0cf601905b81811061064b57506104b9565b5f815560010161063e565b7fb10e2d527612073b26eecdfd717e6a320cf44b4afac2b0732d9fcbe2b7fa0cf69150610614565b9060206003198301126101045760043567ffffffffffffffff811161010457826023820112156101045780600401359267ffffffffffffffff84116101045760248483010111610104576024019190565b9190602083019260038210156106e25752565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52602160045260245ffd5b90600182811c92168015610756575b602083101461072957565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52602260045260245ffd5b91607f169161071e565b7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0601f602060409481855280519182918282880152018686015e5f858286010152011601019056fea164736f6c634300081c000a",
}

// DummyLightClientABI is the input ABI used to generate the binding from.
// Deprecated: Use DummyLightClientMetaData.ABI instead.
var DummyLightClientABI = DummyLightClientMetaData.ABI

// DummyLightClientBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use DummyLightClientMetaData.Bin instead.
var DummyLightClientBin = DummyLightClientMetaData.Bin

// DeployDummyLightClient deploys a new Ethereum contract, binding an instance of DummyLightClient to it.
func DeployDummyLightClient(auth *bind.TransactOpts, backend bind.ContractBackend, updateResult_ uint8, membershipResult_ uint64, membershipShouldFail_ bool) (common.Address, *types.Transaction, *DummyLightClient, error) {
	parsed, err := DummyLightClientMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(DummyLightClientBin), backend, updateResult_, membershipResult_, membershipShouldFail_)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &DummyLightClient{DummyLightClientCaller: DummyLightClientCaller{contract: contract}, DummyLightClientTransactor: DummyLightClientTransactor{contract: contract}, DummyLightClientFilterer: DummyLightClientFilterer{contract: contract}}, nil
}

// DummyLightClient is an auto generated Go binding around an Ethereum contract.
type DummyLightClient struct {
	DummyLightClientCaller     // Read-only binding to the contract
	DummyLightClientTransactor // Write-only binding to the contract
	DummyLightClientFilterer   // Log filterer for contract events
}

// DummyLightClientCaller is an auto generated read-only Go binding around an Ethereum contract.
type DummyLightClientCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DummyLightClientTransactor is an auto generated write-only Go binding around an Ethereum contract.
type DummyLightClientTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DummyLightClientFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type DummyLightClientFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DummyLightClientSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type DummyLightClientSession struct {
	Contract     *DummyLightClient // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// DummyLightClientCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type DummyLightClientCallerSession struct {
	Contract *DummyLightClientCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts           // Call options to use throughout this session
}

// DummyLightClientTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type DummyLightClientTransactorSession struct {
	Contract     *DummyLightClientTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts           // Transaction auth options to use throughout this session
}

// DummyLightClientRaw is an auto generated low-level Go binding around an Ethereum contract.
type DummyLightClientRaw struct {
	Contract *DummyLightClient // Generic contract binding to access the raw methods on
}

// DummyLightClientCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type DummyLightClientCallerRaw struct {
	Contract *DummyLightClientCaller // Generic read-only contract binding to access the raw methods on
}

// DummyLightClientTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type DummyLightClientTransactorRaw struct {
	Contract *DummyLightClientTransactor // Generic write-only contract binding to access the raw methods on
}

// NewDummyLightClient creates a new instance of DummyLightClient, bound to a specific deployed contract.
func NewDummyLightClient(address common.Address, backend bind.ContractBackend) (*DummyLightClient, error) {
	contract, err := bindDummyLightClient(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &DummyLightClient{DummyLightClientCaller: DummyLightClientCaller{contract: contract}, DummyLightClientTransactor: DummyLightClientTransactor{contract: contract}, DummyLightClientFilterer: DummyLightClientFilterer{contract: contract}}, nil
}

// NewDummyLightClientCaller creates a new read-only instance of DummyLightClient, bound to a specific deployed contract.
func NewDummyLightClientCaller(address common.Address, caller bind.ContractCaller) (*DummyLightClientCaller, error) {
	contract, err := bindDummyLightClient(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &DummyLightClientCaller{contract: contract}, nil
}

// NewDummyLightClientTransactor creates a new write-only instance of DummyLightClient, bound to a specific deployed contract.
func NewDummyLightClientTransactor(address common.Address, transactor bind.ContractTransactor) (*DummyLightClientTransactor, error) {
	contract, err := bindDummyLightClient(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &DummyLightClientTransactor{contract: contract}, nil
}

// NewDummyLightClientFilterer creates a new log filterer instance of DummyLightClient, bound to a specific deployed contract.
func NewDummyLightClientFilterer(address common.Address, filterer bind.ContractFilterer) (*DummyLightClientFilterer, error) {
	contract, err := bindDummyLightClient(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &DummyLightClientFilterer{contract: contract}, nil
}

// bindDummyLightClient binds a generic wrapper to an already deployed contract.
func bindDummyLightClient(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := DummyLightClientMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DummyLightClient *DummyLightClientRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DummyLightClient.Contract.DummyLightClientCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DummyLightClient *DummyLightClientRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DummyLightClient.Contract.DummyLightClientTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DummyLightClient *DummyLightClientRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DummyLightClient.Contract.DummyLightClientTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DummyLightClient *DummyLightClientCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DummyLightClient.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DummyLightClient *DummyLightClientTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DummyLightClient.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DummyLightClient *DummyLightClientTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DummyLightClient.Contract.contract.Transact(opts, method, params...)
}

// GetClientState is a free data retrieval call binding the contract method 0xef913a4b.
//
// Solidity: function getClientState() pure returns(bytes)
func (_DummyLightClient *DummyLightClientCaller) GetClientState(opts *bind.CallOpts) ([]byte, error) {
	var out []interface{}
	err := _DummyLightClient.contract.Call(opts, &out, "getClientState")

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// GetClientState is a free data retrieval call binding the contract method 0xef913a4b.
//
// Solidity: function getClientState() pure returns(bytes)
func (_DummyLightClient *DummyLightClientSession) GetClientState() ([]byte, error) {
	return _DummyLightClient.Contract.GetClientState(&_DummyLightClient.CallOpts)
}

// GetClientState is a free data retrieval call binding the contract method 0xef913a4b.
//
// Solidity: function getClientState() pure returns(bytes)
func (_DummyLightClient *DummyLightClientCallerSession) GetClientState() ([]byte, error) {
	return _DummyLightClient.Contract.GetClientState(&_DummyLightClient.CallOpts)
}

// LatestUpdateMsg is a free data retrieval call binding the contract method 0xe6fbdbf0.
//
// Solidity: function latestUpdateMsg() view returns(bytes)
func (_DummyLightClient *DummyLightClientCaller) LatestUpdateMsg(opts *bind.CallOpts) ([]byte, error) {
	var out []interface{}
	err := _DummyLightClient.contract.Call(opts, &out, "latestUpdateMsg")

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// LatestUpdateMsg is a free data retrieval call binding the contract method 0xe6fbdbf0.
//
// Solidity: function latestUpdateMsg() view returns(bytes)
func (_DummyLightClient *DummyLightClientSession) LatestUpdateMsg() ([]byte, error) {
	return _DummyLightClient.Contract.LatestUpdateMsg(&_DummyLightClient.CallOpts)
}

// LatestUpdateMsg is a free data retrieval call binding the contract method 0xe6fbdbf0.
//
// Solidity: function latestUpdateMsg() view returns(bytes)
func (_DummyLightClient *DummyLightClientCallerSession) LatestUpdateMsg() ([]byte, error) {
	return _DummyLightClient.Contract.LatestUpdateMsg(&_DummyLightClient.CallOpts)
}

// MembershipResult is a free data retrieval call binding the contract method 0x155be2af.
//
// Solidity: function membershipResult() view returns(uint64)
func (_DummyLightClient *DummyLightClientCaller) MembershipResult(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _DummyLightClient.contract.Call(opts, &out, "membershipResult")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// MembershipResult is a free data retrieval call binding the contract method 0x155be2af.
//
// Solidity: function membershipResult() view returns(uint64)
func (_DummyLightClient *DummyLightClientSession) MembershipResult() (uint64, error) {
	return _DummyLightClient.Contract.MembershipResult(&_DummyLightClient.CallOpts)
}

// MembershipResult is a free data retrieval call binding the contract method 0x155be2af.
//
// Solidity: function membershipResult() view returns(uint64)
func (_DummyLightClient *DummyLightClientCallerSession) MembershipResult() (uint64, error) {
	return _DummyLightClient.Contract.MembershipResult(&_DummyLightClient.CallOpts)
}

// MembershipShouldFail is a free data retrieval call binding the contract method 0x374abcc9.
//
// Solidity: function membershipShouldFail() view returns(bool)
func (_DummyLightClient *DummyLightClientCaller) MembershipShouldFail(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _DummyLightClient.contract.Call(opts, &out, "membershipShouldFail")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// MembershipShouldFail is a free data retrieval call binding the contract method 0x374abcc9.
//
// Solidity: function membershipShouldFail() view returns(bool)
func (_DummyLightClient *DummyLightClientSession) MembershipShouldFail() (bool, error) {
	return _DummyLightClient.Contract.MembershipShouldFail(&_DummyLightClient.CallOpts)
}

// MembershipShouldFail is a free data retrieval call binding the contract method 0x374abcc9.
//
// Solidity: function membershipShouldFail() view returns(bool)
func (_DummyLightClient *DummyLightClientCallerSession) MembershipShouldFail() (bool, error) {
	return _DummyLightClient.Contract.MembershipShouldFail(&_DummyLightClient.CallOpts)
}

// UpdateResult is a free data retrieval call binding the contract method 0x80ebb08e.
//
// Solidity: function updateResult() view returns(uint8)
func (_DummyLightClient *DummyLightClientCaller) UpdateResult(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _DummyLightClient.contract.Call(opts, &out, "updateResult")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// UpdateResult is a free data retrieval call binding the contract method 0x80ebb08e.
//
// Solidity: function updateResult() view returns(uint8)
func (_DummyLightClient *DummyLightClientSession) UpdateResult() (uint8, error) {
	return _DummyLightClient.Contract.UpdateResult(&_DummyLightClient.CallOpts)
}

// UpdateResult is a free data retrieval call binding the contract method 0x80ebb08e.
//
// Solidity: function updateResult() view returns(uint8)
func (_DummyLightClient *DummyLightClientCallerSession) UpdateResult() (uint8, error) {
	return _DummyLightClient.Contract.UpdateResult(&_DummyLightClient.CallOpts)
}

// VerifyMembership is a free data retrieval call binding the contract method 0x682ed5f0.
//
// Solidity: function verifyMembership((bytes,(uint64,uint64),bytes[],bytes) ) view returns(uint256)
func (_DummyLightClient *DummyLightClientCaller) VerifyMembership(opts *bind.CallOpts, arg0 ILightClientMsgsMsgVerifyMembership) (*big.Int, error) {
	var out []interface{}
	err := _DummyLightClient.contract.Call(opts, &out, "verifyMembership", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// VerifyMembership is a free data retrieval call binding the contract method 0x682ed5f0.
//
// Solidity: function verifyMembership((bytes,(uint64,uint64),bytes[],bytes) ) view returns(uint256)
func (_DummyLightClient *DummyLightClientSession) VerifyMembership(arg0 ILightClientMsgsMsgVerifyMembership) (*big.Int, error) {
	return _DummyLightClient.Contract.VerifyMembership(&_DummyLightClient.CallOpts, arg0)
}

// VerifyMembership is a free data retrieval call binding the contract method 0x682ed5f0.
//
// Solidity: function verifyMembership((bytes,(uint64,uint64),bytes[],bytes) ) view returns(uint256)
func (_DummyLightClient *DummyLightClientCallerSession) VerifyMembership(arg0 ILightClientMsgsMsgVerifyMembership) (*big.Int, error) {
	return _DummyLightClient.Contract.VerifyMembership(&_DummyLightClient.CallOpts, arg0)
}

// VerifyNonMembership is a free data retrieval call binding the contract method 0x4d6d9ffb.
//
// Solidity: function verifyNonMembership((bytes,(uint64,uint64),bytes[]) ) view returns(uint256)
func (_DummyLightClient *DummyLightClientCaller) VerifyNonMembership(opts *bind.CallOpts, arg0 ILightClientMsgsMsgVerifyNonMembership) (*big.Int, error) {
	var out []interface{}
	err := _DummyLightClient.contract.Call(opts, &out, "verifyNonMembership", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// VerifyNonMembership is a free data retrieval call binding the contract method 0x4d6d9ffb.
//
// Solidity: function verifyNonMembership((bytes,(uint64,uint64),bytes[]) ) view returns(uint256)
func (_DummyLightClient *DummyLightClientSession) VerifyNonMembership(arg0 ILightClientMsgsMsgVerifyNonMembership) (*big.Int, error) {
	return _DummyLightClient.Contract.VerifyNonMembership(&_DummyLightClient.CallOpts, arg0)
}

// VerifyNonMembership is a free data retrieval call binding the contract method 0x4d6d9ffb.
//
// Solidity: function verifyNonMembership((bytes,(uint64,uint64),bytes[]) ) view returns(uint256)
func (_DummyLightClient *DummyLightClientCallerSession) VerifyNonMembership(arg0 ILightClientMsgsMsgVerifyNonMembership) (*big.Int, error) {
	return _DummyLightClient.Contract.VerifyNonMembership(&_DummyLightClient.CallOpts, arg0)
}

// Misbehaviour is a paid mutator transaction binding the contract method 0xddba6537.
//
// Solidity: function misbehaviour(bytes misbehaviourMsg) returns()
func (_DummyLightClient *DummyLightClientTransactor) Misbehaviour(opts *bind.TransactOpts, misbehaviourMsg []byte) (*types.Transaction, error) {
	return _DummyLightClient.contract.Transact(opts, "misbehaviour", misbehaviourMsg)
}

// Misbehaviour is a paid mutator transaction binding the contract method 0xddba6537.
//
// Solidity: function misbehaviour(bytes misbehaviourMsg) returns()
func (_DummyLightClient *DummyLightClientSession) Misbehaviour(misbehaviourMsg []byte) (*types.Transaction, error) {
	return _DummyLightClient.Contract.Misbehaviour(&_DummyLightClient.TransactOpts, misbehaviourMsg)
}

// Misbehaviour is a paid mutator transaction binding the contract method 0xddba6537.
//
// Solidity: function misbehaviour(bytes misbehaviourMsg) returns()
func (_DummyLightClient *DummyLightClientTransactorSession) Misbehaviour(misbehaviourMsg []byte) (*types.Transaction, error) {
	return _DummyLightClient.Contract.Misbehaviour(&_DummyLightClient.TransactOpts, misbehaviourMsg)
}

// SetMembershipResult is a paid mutator transaction binding the contract method 0xd619aedd.
//
// Solidity: function setMembershipResult(uint64 membershipResult_, bool shouldFail) returns()
func (_DummyLightClient *DummyLightClientTransactor) SetMembershipResult(opts *bind.TransactOpts, membershipResult_ uint64, shouldFail bool) (*types.Transaction, error) {
	return _DummyLightClient.contract.Transact(opts, "setMembershipResult", membershipResult_, shouldFail)
}

// SetMembershipResult is a paid mutator transaction binding the contract method 0xd619aedd.
//
// Solidity: function setMembershipResult(uint64 membershipResult_, bool shouldFail) returns()
func (_DummyLightClient *DummyLightClientSession) SetMembershipResult(membershipResult_ uint64, shouldFail bool) (*types.Transaction, error) {
	return _DummyLightClient.Contract.SetMembershipResult(&_DummyLightClient.TransactOpts, membershipResult_, shouldFail)
}

// SetMembershipResult is a paid mutator transaction binding the contract method 0xd619aedd.
//
// Solidity: function setMembershipResult(uint64 membershipResult_, bool shouldFail) returns()
func (_DummyLightClient *DummyLightClientTransactorSession) SetMembershipResult(membershipResult_ uint64, shouldFail bool) (*types.Transaction, error) {
	return _DummyLightClient.Contract.SetMembershipResult(&_DummyLightClient.TransactOpts, membershipResult_, shouldFail)
}

// SetUpdateResult is a paid mutator transaction binding the contract method 0xc20fd8b4.
//
// Solidity: function setUpdateResult(uint8 updateResult_) returns()
func (_DummyLightClient *DummyLightClientTransactor) SetUpdateResult(opts *bind.TransactOpts, updateResult_ uint8) (*types.Transaction, error) {
	return _DummyLightClient.contract.Transact(opts, "setUpdateResult", updateResult_)
}

// SetUpdateResult is a paid mutator transaction binding the contract method 0xc20fd8b4.
//
// Solidity: function setUpdateResult(uint8 updateResult_) returns()
func (_DummyLightClient *DummyLightClientSession) SetUpdateResult(updateResult_ uint8) (*types.Transaction, error) {
	return _DummyLightClient.Contract.SetUpdateResult(&_DummyLightClient.TransactOpts, updateResult_)
}

// SetUpdateResult is a paid mutator transaction binding the contract method 0xc20fd8b4.
//
// Solidity: function setUpdateResult(uint8 updateResult_) returns()
func (_DummyLightClient *DummyLightClientTransactorSession) SetUpdateResult(updateResult_ uint8) (*types.Transaction, error) {
	return _DummyLightClient.Contract.SetUpdateResult(&_DummyLightClient.TransactOpts, updateResult_)
}

// UpdateClient is a paid mutator transaction binding the contract method 0x0bece356.
//
// Solidity: function updateClient(bytes updateMsg) returns(uint8)
func (_DummyLightClient *DummyLightClientTransactor) UpdateClient(opts *bind.TransactOpts, updateMsg []byte) (*types.Transaction, error) {
	return _DummyLightClient.contract.Transact(opts, "updateClient", updateMsg)
}

// UpdateClient is a paid mutator transaction binding the contract method 0x0bece356.
//
// Solidity: function updateClient(bytes updateMsg) returns(uint8)
func (_DummyLightClient *DummyLightClientSession) UpdateClient(updateMsg []byte) (*types.Transaction, error) {
	return _DummyLightClient.Contract.UpdateClient(&_DummyLightClient.TransactOpts, updateMsg)
}

// UpdateClient is a paid mutator transaction binding the contract method 0x0bece356.
//
// Solidity: function updateClient(bytes updateMsg) returns(uint8)
func (_DummyLightClient *DummyLightClientTransactorSession) UpdateClient(updateMsg []byte) (*types.Transaction, error) {
	return _DummyLightClient.Contract.UpdateClient(&_DummyLightClient.TransactOpts, updateMsg)
}
