// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package ether

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

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
	_ = time.Tick
	_ = context.Background
)

// RegistryEntry is an auto generated low-level Go binding around an user-defined struct.
type RegistryEntry struct {
	Platform    string
	Service     string
	Version     string
	Measurement [32]byte
	Expires     uint64
}

// RegistryMetaData contains all meta data concerning the Registry contract.
var RegistryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"approver\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"APPROVER\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"digest\",\"inputs\":[{\"name\":\"e\",\"type\":\"tuple\",\"internalType\":\"structRegistry.Entry\",\"components\":[{\"name\":\"platform\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"service\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"measurement\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"expires\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"keyFor\",\"inputs\":[{\"name\":\"platform\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"measurement\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"keyOf\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"platform\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"measurement\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"lookup\",\"inputs\":[{\"name\":\"key\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"expires\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"registeredAt\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"chainTime\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"records\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"expires\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"registeredAt\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"register\",\"inputs\":[{\"name\":\"e\",\"type\":\"tuple\",\"internalType\":\"structRegistry.Entry\",\"components\":[{\"name\":\"platform\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"service\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"measurement\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"expires\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"key\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"EIP712DomainChanged\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Registered\",\"inputs\":[{\"name\":\"key\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"signer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"registrant\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"platform\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"service\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"measurement\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"expires\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"},{\"name\":\"signature\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AlreadyRegistered\",\"inputs\":[{\"name\":\"key\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"expires\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"ECDSAInvalidSignature\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ECDSAInvalidSignatureLength\",\"inputs\":[{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ECDSAInvalidSignatureS\",\"inputs\":[{\"name\":\"s\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"ExpiryAlreadyPassed\",\"inputs\":[{\"name\":\"expires\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"nowTime\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"InvalidShortString\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MissingField\",\"inputs\":[{\"name\":\"field\",\"type\":\"string\",\"internalType\":\"string\"}]},{\"type\":\"error\",\"name\":\"NotAnApprover\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"StringTooLong\",\"inputs\":[{\"name\":\"str\",\"type\":\"string\",\"internalType\":\"string\"}]}]",
	Bin: "0x610180604052348015610010575f5ffd5b506040516112dd3803806112dd83398101604081905261002f916101b1565b60405180604001604052806008815260200167526567697374727960c01b815250604051806040016040528060018152602001603160f81b8152506100798261017460201b60201c565b6101205261008681610174565b61014052815160208084019190912060e052815190820120610100524660a05261011260e05161010051604080517f8b73c3c69bb8fe3d512ecc4cf759cc79239f7b179b0ffacaa9a75d522b39400f60208201529081019290925260608201524660808201523060a08201525f9060c00160405160208183030381529060405280519060200120905090565b60805250503060c0526001600160a01b0381166101625760405163cdda309b60e01b815260206004820152600860248201526730b8383937bb32b960c11b60448201526064015b60405180910390fd5b6001600160a01b031661016052610239565b5f5f829050601f8151111561019e578260405163305a27a960e01b815260040161015991906101de565b80516101a982610213565b179392505050565b5f602082840312156101c1575f5ffd5b81516001600160a01b03811681146101d7575f5ffd5b9392505050565b602081525f82518060208401528060208501604085015e5f604082850101526040601f19601f83011684010191505092915050565b80516020808301519190811015610233575f198160200360031b1b821691505b50919050565b60805160a05160c05160e0516101005161012051610140516101605161103b6102a25f395f818161013b015281816105af015261078401525f6108cf01525f61089f01525f6109cc01525f6109a401525f6108ff01525f61092901525f610953015261103b5ff3fe608060405234801561000f575f5ffd5b5060043610610085575f3560e01c806346e9ce7c1161005857806346e9ce7c1461017557806384b0196e14610188578063ea254739146101a3578063f39ec1f7146101b6575f5ffd5b806301e647251461008957806323294977146101025780633bde48e9146101235780633d111c7e14610136575b5f5ffd5b6100ce610097366004610c4d565b60026020525f9081526040902080546001909101546001600160a01b038216916001600160401b03600160a01b9091048116911683565b604080516001600160a01b0390941684526001600160401b0392831660208501529116908201526060015b60405180910390f35b610115610110366004610c7a565b610238565b6040519081526020016100f9565b610115610131366004610cef565b610333565b61015d7f000000000000000000000000000000000000000000000000000000000000000081565b6040516001600160a01b0390911681526020016100f9565b610115610183366004610d51565b61038f565b61019061073c565b6040516100f99796959493929190610de6565b6101156101b1366004610e7c565b61077e565b6101ff6101c4366004610c4d565b5f90815260026020526040902080546001909101546001600160a01b03821692600160a01b9092046001600160401b03908116929116904290565b604080516001600160a01b0390951685526001600160401b039384166020860152918316918401919091521660608201526080016100f9565b5f61032d7fb5a90ef326d9c24f8dc84d8096c40336c9778fe5d31123f332e81bde23137efe6102678480610ec3565b604051610275929190610f05565b60405190819003902061028b6020860186610ec3565b604051610299929190610f05565b60405180910390208580604001906102b19190610ec3565b6040516102bf929190610f05565b60405190819003902060608701356102dd60a0890160808a01610f14565b6040805160208101979097528601949094526060850192909252608084015260a08301526001600160401b031660c082015260e001604051602081830303815290604052805190602001206107b3565b92915050565b5f848484604051610345929190610f05565b604080519182900382206001600160a01b03909316602083015281019190915260608101839052608001604051602081830303815290604052805190602001209050949350505050565b5f61039a8480610ec3565b90505f036103db5760405163cdda309b60e01b8152602060048201526008602482015267706c6174666f726d60c01b60448201526064015b60405180910390fd5b6103e86020850185610ec3565b90505f036104235760405163cdda309b60e01b81526020600482015260076024820152667365727669636560c81b60448201526064016103d2565b6104306040850185610ec3565b90505f0361046b5760405163cdda309b60e01b81526020600482015260076024820152663b32b939b4b7b760c91b60448201526064016103d2565b60608401356104ab5760405163cdda309b60e01b815260206004820152600b60248201526a1b59585cdd5c995b595b9d60aa1b60448201526064016103d2565b426104bc60a0860160808701610f14565b6001600160401b03165f036104fe5760405163cdda309b60e01b81526020600482015260076024820152666578706972657360c81b60448201526064016103d2565b6001600160401b03811661051860a0870160808801610f14565b6001600160401b0316116105635761053660a0860160808701610f14565b60405163c27b2b5160e01b81526001600160401b03918216600482015290821660248201526044016103d2565b5f6105ab61057087610238565b86868080601f0160208091040260200160405190810160405280939291908181526020018383808284375f920191909152506107df92505050565b90507f00000000000000000000000000000000000000000000000000000000000000006001600160a01b0316816001600160a01b03161461060a576040516364b4a12160e11b81526001600160a01b03821660048201526024016103d2565b610622816106188880610ec3565b8960600135610333565b5f81815260026020526040902054909350600160a01b90046001600160401b031615610689575f83815260026020526040908190205490516326094cb760e11b815260048101859052600160a01b9091046001600160401b031660248201526044016103d2565b60408051606081019091526001600160a01b0382168152602081016106b460a0890160808a01610f14565b6001600160401b0390811682528481166020928301525f8681526002835260409081902084518154948601518416600160a01b026001600160e01b03199095166001600160a01b03909116179390931783559290920151600190910180549190921667ffffffffffffffff199091161790556107338382888888610807565b50509392505050565b5f6060805f5f5f606061074d610898565b6107556108c8565b604080515f80825260208201909252600f60f81b9b939a50919850469750309650945092509050565b5f6107ab7f0000000000000000000000000000000000000000000000000000000000000000858585610333565b949350505050565b5f61032d6107bf6108f3565b8360405161190160f01b8152600281019290925260228201526042902090565b5f5f5f5f6107ed8686610a1c565b9250925092506107fd8282610a65565b5090949350505050565b6001600160a01b038416857fba44e98c24a9fecda812bd8a5b3525cb7f075f8de749aea2440e34b3573b50043361083e8780610ec3565b61084b60208a018a610ec3565b61085860408c018c610ec3565b8c606001358d60800160208101906108709190610f14565b8d8d6040516108899b9a99989796959493929190610f69565b60405180910390a35050505050565b60606108c37f0000000000000000000000000000000000000000000000000000000000000000610b21565b905090565b60606108c37f0000000000000000000000000000000000000000000000000000000000000000610b21565b5f306001600160a01b037f00000000000000000000000000000000000000000000000000000000000000001614801561094b57507f000000000000000000000000000000000000000000000000000000000000000046145b1561097557507f000000000000000000000000000000000000000000000000000000000000000090565b6108c3604080517f8b73c3c69bb8fe3d512ecc4cf759cc79239f7b179b0ffacaa9a75d522b39400f60208201527f0000000000000000000000000000000000000000000000000000000000000000918101919091527f000000000000000000000000000000000000000000000000000000000000000060608201524660808201523060a08201525f9060c00160405160208183030381529060405280519060200120905090565b5f5f5f8351604103610a53576020840151604085015160608601515f1a610a4588828585610b5e565b955095509550505050610a5e565b505081515f91506002905b9250925092565b5f826003811115610a7857610a78610ff1565b03610a81575050565b6001826003811115610a9557610a95610ff1565b03610ab35760405163f645eedf60e01b815260040160405180910390fd5b6002826003811115610ac757610ac7610ff1565b03610ae85760405163fce698f760e01b8152600481018290526024016103d2565b6003826003811115610afc57610afc610ff1565b03610b1d576040516335e2f38360e21b8152600481018290526024016103d2565b5050565b60605f610b2d83610c26565b6040805160208082528183019092529192505f91906020820181803683375050509182525060208101929092525090565b5f80807f7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0841115610b9757505f91506003905082610c1c565b604080515f808252602082018084528a905260ff891692820192909252606081018790526080810186905260019060a0016020604051602081039080840390855afa158015610be8573d5f5f3e3d5ffd5b5050604051601f1901519150506001600160a01b038116610c1357505f925060019150829050610c1c565b92505f91508190505b9450945094915050565b5f60ff8216601f81111561032d57604051632cd44ac360e21b815260040160405180910390fd5b5f60208284031215610c5d575f5ffd5b5035919050565b5f60a08284031215610c74575f5ffd5b50919050565b5f60208284031215610c8a575f5ffd5b81356001600160401b03811115610c9f575f5ffd5b6107ab84828501610c64565b5f5f83601f840112610cbb575f5ffd5b5081356001600160401b03811115610cd1575f5ffd5b602083019150836020828501011115610ce8575f5ffd5b9250929050565b5f5f5f5f60608587031215610d02575f5ffd5b84356001600160a01b0381168114610d18575f5ffd5b935060208501356001600160401b03811115610d32575f5ffd5b610d3e87828801610cab565b9598909750949560400135949350505050565b5f5f5f60408486031215610d63575f5ffd5b83356001600160401b03811115610d78575f5ffd5b610d8486828701610c64565b93505060208401356001600160401b03811115610d9f575f5ffd5b610dab86828701610cab565b9497909650939450505050565b5f81518084528060208401602086015e5f602082860101526020601f19601f83011685010191505092915050565b60ff60f81b8816815260e060208201525f610e0460e0830189610db8565b8281036040840152610e168189610db8565b606084018890526001600160a01b038716608085015260a0840186905283810360c0850152845180825260208087019350909101905f5b81811015610e6b578351835260209384019390920191600101610e4d565b50909b9a5050505050505050505050565b5f5f5f60408486031215610e8e575f5ffd5b83356001600160401b03811115610ea3575f5ffd5b610eaf86828701610cab565b909790965060209590950135949350505050565b5f5f8335601e19843603018112610ed8575f5ffd5b8301803591506001600160401b03821115610ef1575f5ffd5b602001915036819003821315610ce8575f5ffd5b818382375f9101908152919050565b5f60208284031215610f24575f5ffd5b81356001600160401b0381168114610f3a575f5ffd5b9392505050565b81835281816020850137505f828201602090810191909152601f909101601f19169091010190565b6001600160a01b038c16815260e0602082018190525f90610f8d9083018c8e610f41565b8281036040840152610fa0818b8d610f41565b90508281036060840152610fb581898b610f41565b90508660808401526001600160401b03861660a084015282810360c0840152610fdf818587610f41565b9e9d5050505050505050505050505050565b634e487b7160e01b5f52602160045260245ffdfea26469706673582212201b770d08e523f39037478b4b3009cd87a8e14372a16fa182d4f0f739e9de0bf764736f6c634300081c0033",
}

// RegistryABI is the input ABI used to generate the binding from.
// Deprecated: Use RegistryMetaData.ABI instead.
var RegistryABI = RegistryMetaData.ABI

// RegistryBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use RegistryMetaData.Bin instead.
var RegistryBin = RegistryMetaData.Bin

// DeployRegistry deploys a new Ethereum contract, binding an instance of Registry to it.
func DeployRegistry(auth *bind.TransactOpts, backend bind.ContractBackend, approver common.Address) (common.Address, *types.Transaction, *Registry, error) {
	parsed, err := RegistryMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(RegistryBin), backend, approver)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &Registry{RegistryCaller: RegistryCaller{contract: contract}, RegistryTransactor: RegistryTransactor{contract: contract}, RegistryFilterer: RegistryFilterer{contract: contract}}, nil
}

// Registry is an auto generated Go binding around an Ethereum contract.
type Registry struct {
	RegistryCaller     // Read-only binding to the contract
	RegistryTransactor // Write-only binding to the contract
	RegistryFilterer   // Log filterer for contract events
}

// RegistryCaller is an auto generated read-only Go binding around an Ethereum contract.
type RegistryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type RegistryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type RegistryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RegistrySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type RegistrySession struct {
	Contract     *Registry         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// RegistryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type RegistryCallerSession struct {
	Contract *RegistryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// RegistryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type RegistryTransactorSession struct {
	Contract     *RegistryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// RegistryRaw is an auto generated low-level Go binding around an Ethereum contract.
type RegistryRaw struct {
	Contract *Registry // Generic contract binding to access the raw methods on
}

// RegistryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type RegistryCallerRaw struct {
	Contract *RegistryCaller // Generic read-only contract binding to access the raw methods on
}

// RegistryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type RegistryTransactorRaw struct {
	Contract *RegistryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewRegistry creates a new instance of Registry, bound to a specific deployed contract.
func NewRegistry(address common.Address, backend bind.ContractBackend) (*Registry, error) {
	contract, err := bindRegistry(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Registry{RegistryCaller: RegistryCaller{contract: contract}, RegistryTransactor: RegistryTransactor{contract: contract}, RegistryFilterer: RegistryFilterer{contract: contract}}, nil
}

// NewRegistryCaller creates a new read-only instance of Registry, bound to a specific deployed contract.
func NewRegistryCaller(address common.Address, caller bind.ContractCaller) (*RegistryCaller, error) {
	contract, err := bindRegistry(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &RegistryCaller{contract: contract}, nil
}

// NewRegistryTransactor creates a new write-only instance of Registry, bound to a specific deployed contract.
func NewRegistryTransactor(address common.Address, transactor bind.ContractTransactor) (*RegistryTransactor, error) {
	contract, err := bindRegistry(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &RegistryTransactor{contract: contract}, nil
}

// NewRegistryFilterer creates a new log filterer instance of Registry, bound to a specific deployed contract.
func NewRegistryFilterer(address common.Address, filterer bind.ContractFilterer) (*RegistryFilterer, error) {
	contract, err := bindRegistry(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &RegistryFilterer{contract: contract}, nil
}

// bindRegistry binds a generic wrapper to an already deployed contract.
func bindRegistry(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := RegistryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Registry *RegistryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Registry.Contract.RegistryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Registry *RegistryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Registry.Contract.RegistryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Registry *RegistryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Registry.Contract.RegistryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Registry *RegistryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Registry.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Registry *RegistryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Registry.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Registry *RegistryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Registry.Contract.contract.Transact(opts, method, params...)
}

// APPROVER is a free data retrieval call binding the contract method 0x3d111c7e.
//
// Solidity: function APPROVER() view returns(address)
func (_Registry *RegistryCaller) APPROVER(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "APPROVER")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// APPROVER is a free data retrieval call binding the contract method 0x3d111c7e.
//
// Solidity: function APPROVER() view returns(address)
func (_Registry *RegistrySession) APPROVER() (common.Address, error) {
	return _Registry.Contract.APPROVER(&_Registry.CallOpts)
}

// APPROVER is a free data retrieval call binding the contract method 0x3d111c7e.
//
// Solidity: function APPROVER() view returns(address)
func (_Registry *RegistryCallerSession) APPROVER() (common.Address, error) {
	return _Registry.Contract.APPROVER(&_Registry.CallOpts)
}

// Digest is a free data retrieval call binding the contract method 0x23294977.
//
// Solidity: function digest((string,string,string,bytes32,uint64) e) view returns(bytes32)
func (_Registry *RegistryCaller) Digest(opts *bind.CallOpts, e RegistryEntry) ([32]byte, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "digest", e)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// Digest is a free data retrieval call binding the contract method 0x23294977.
//
// Solidity: function digest((string,string,string,bytes32,uint64) e) view returns(bytes32)
func (_Registry *RegistrySession) Digest(e RegistryEntry) ([32]byte, error) {
	return _Registry.Contract.Digest(&_Registry.CallOpts, e)
}

// Digest is a free data retrieval call binding the contract method 0x23294977.
//
// Solidity: function digest((string,string,string,bytes32,uint64) e) view returns(bytes32)
func (_Registry *RegistryCallerSession) Digest(e RegistryEntry) ([32]byte, error) {
	return _Registry.Contract.Digest(&_Registry.CallOpts, e)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_Registry *RegistryCaller) Eip712Domain(opts *bind.CallOpts) (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "eip712Domain")

	outstruct := new(struct {
		Fields            [1]byte
		Name              string
		Version           string
		ChainId           *big.Int
		VerifyingContract common.Address
		Salt              [32]byte
		Extensions        []*big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Fields = *abi.ConvertType(out[0], new([1]byte)).(*[1]byte)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.Version = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.ChainId = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.VerifyingContract = *abi.ConvertType(out[4], new(common.Address)).(*common.Address)
	outstruct.Salt = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Extensions = *abi.ConvertType(out[6], new([]*big.Int)).(*[]*big.Int)

	return *outstruct, err

}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_Registry *RegistrySession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _Registry.Contract.Eip712Domain(&_Registry.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_Registry *RegistryCallerSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _Registry.Contract.Eip712Domain(&_Registry.CallOpts)
}

// KeyFor is a free data retrieval call binding the contract method 0xea254739.
//
// Solidity: function keyFor(string platform, bytes32 measurement) view returns(bytes32)
func (_Registry *RegistryCaller) KeyFor(opts *bind.CallOpts, platform string, measurement [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "keyFor", platform, measurement)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// KeyFor is a free data retrieval call binding the contract method 0xea254739.
//
// Solidity: function keyFor(string platform, bytes32 measurement) view returns(bytes32)
func (_Registry *RegistrySession) KeyFor(platform string, measurement [32]byte) ([32]byte, error) {
	return _Registry.Contract.KeyFor(&_Registry.CallOpts, platform, measurement)
}

// KeyFor is a free data retrieval call binding the contract method 0xea254739.
//
// Solidity: function keyFor(string platform, bytes32 measurement) view returns(bytes32)
func (_Registry *RegistryCallerSession) KeyFor(platform string, measurement [32]byte) ([32]byte, error) {
	return _Registry.Contract.KeyFor(&_Registry.CallOpts, platform, measurement)
}

// KeyOf is a free data retrieval call binding the contract method 0x3bde48e9.
//
// Solidity: function keyOf(address signer, string platform, bytes32 measurement) pure returns(bytes32)
func (_Registry *RegistryCaller) KeyOf(opts *bind.CallOpts, signer common.Address, platform string, measurement [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "keyOf", signer, platform, measurement)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// KeyOf is a free data retrieval call binding the contract method 0x3bde48e9.
//
// Solidity: function keyOf(address signer, string platform, bytes32 measurement) pure returns(bytes32)
func (_Registry *RegistrySession) KeyOf(signer common.Address, platform string, measurement [32]byte) ([32]byte, error) {
	return _Registry.Contract.KeyOf(&_Registry.CallOpts, signer, platform, measurement)
}

// KeyOf is a free data retrieval call binding the contract method 0x3bde48e9.
//
// Solidity: function keyOf(address signer, string platform, bytes32 measurement) pure returns(bytes32)
func (_Registry *RegistryCallerSession) KeyOf(signer common.Address, platform string, measurement [32]byte) ([32]byte, error) {
	return _Registry.Contract.KeyOf(&_Registry.CallOpts, signer, platform, measurement)
}

// Lookup is a free data retrieval call binding the contract method 0xf39ec1f7.
//
// Solidity: function lookup(bytes32 key) view returns(address signer, uint64 expires, uint64 registeredAt, uint64 chainTime)
func (_Registry *RegistryCaller) Lookup(opts *bind.CallOpts, key [32]byte) (struct {
	Signer       common.Address
	Expires      uint64
	RegisteredAt uint64
	ChainTime    uint64
}, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "lookup", key)

	outstruct := new(struct {
		Signer       common.Address
		Expires      uint64
		RegisteredAt uint64
		ChainTime    uint64
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Signer = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.Expires = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.RegisteredAt = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.ChainTime = *abi.ConvertType(out[3], new(uint64)).(*uint64)

	return *outstruct, err

}

// Lookup is a free data retrieval call binding the contract method 0xf39ec1f7.
//
// Solidity: function lookup(bytes32 key) view returns(address signer, uint64 expires, uint64 registeredAt, uint64 chainTime)
func (_Registry *RegistrySession) Lookup(key [32]byte) (struct {
	Signer       common.Address
	Expires      uint64
	RegisteredAt uint64
	ChainTime    uint64
}, error) {
	return _Registry.Contract.Lookup(&_Registry.CallOpts, key)
}

// Lookup is a free data retrieval call binding the contract method 0xf39ec1f7.
//
// Solidity: function lookup(bytes32 key) view returns(address signer, uint64 expires, uint64 registeredAt, uint64 chainTime)
func (_Registry *RegistryCallerSession) Lookup(key [32]byte) (struct {
	Signer       common.Address
	Expires      uint64
	RegisteredAt uint64
	ChainTime    uint64
}, error) {
	return _Registry.Contract.Lookup(&_Registry.CallOpts, key)
}

// Records is a free data retrieval call binding the contract method 0x01e64725.
//
// Solidity: function records(bytes32 ) view returns(address signer, uint64 expires, uint64 registeredAt)
func (_Registry *RegistryCaller) Records(opts *bind.CallOpts, arg0 [32]byte) (struct {
	Signer       common.Address
	Expires      uint64
	RegisteredAt uint64
}, error) {
	var out []interface{}
	err := _Registry.contract.Call(opts, &out, "records", arg0)

	outstruct := new(struct {
		Signer       common.Address
		Expires      uint64
		RegisteredAt uint64
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Signer = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.Expires = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.RegisteredAt = *abi.ConvertType(out[2], new(uint64)).(*uint64)

	return *outstruct, err

}

// Records is a free data retrieval call binding the contract method 0x01e64725.
//
// Solidity: function records(bytes32 ) view returns(address signer, uint64 expires, uint64 registeredAt)
func (_Registry *RegistrySession) Records(arg0 [32]byte) (struct {
	Signer       common.Address
	Expires      uint64
	RegisteredAt uint64
}, error) {
	return _Registry.Contract.Records(&_Registry.CallOpts, arg0)
}

// Records is a free data retrieval call binding the contract method 0x01e64725.
//
// Solidity: function records(bytes32 ) view returns(address signer, uint64 expires, uint64 registeredAt)
func (_Registry *RegistryCallerSession) Records(arg0 [32]byte) (struct {
	Signer       common.Address
	Expires      uint64
	RegisteredAt uint64
}, error) {
	return _Registry.Contract.Records(&_Registry.CallOpts, arg0)
}

// Register is a paid mutator transaction binding the contract method 0x46e9ce7c.
//
// Solidity: function register((string,string,string,bytes32,uint64) e, bytes signature) returns(bytes32 key)
func (_Registry *RegistryTransactor) Register(opts *bind.TransactOpts, e RegistryEntry, signature []byte) (*types.Transaction, error) {
	return _Registry.contract.Transact(opts, "register", e, signature)
}

// Register is a paid mutator transaction binding the contract method 0x46e9ce7c.
//
// Solidity: function register((string,string,string,bytes32,uint64) e, bytes signature) returns(bytes32 key)
func (_Registry *RegistrySession) Register(e RegistryEntry, signature []byte) (*types.Transaction, error) {
	return _Registry.Contract.Register(&_Registry.TransactOpts, e, signature)
}

// Register is a paid mutator transaction binding the contract method 0x46e9ce7c.
//
// Solidity: function register((string,string,string,bytes32,uint64) e, bytes signature) returns(bytes32 key)
func (_Registry *RegistryTransactorSession) Register(e RegistryEntry, signature []byte) (*types.Transaction, error) {
	return _Registry.Contract.Register(&_Registry.TransactOpts, e, signature)
}

// RegistryEIP712DomainChangedIterator is returned from FilterEIP712DomainChanged and is used to iterate over the raw logs and unpacked data for EIP712DomainChanged events raised by the Registry contract.
type RegistryEIP712DomainChangedIterator struct {
	Event *RegistryEIP712DomainChanged // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *RegistryEIP712DomainChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RegistryEIP712DomainChanged)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(RegistryEIP712DomainChanged)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *RegistryEIP712DomainChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RegistryEIP712DomainChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RegistryEIP712DomainChanged represents a EIP712DomainChanged event raised by the Registry contract.
type RegistryEIP712DomainChanged struct {
	Raw types.Log // Blockchain specific contextual infos
}

// FilterEIP712DomainChanged is a free log retrieval operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_Registry *RegistryFilterer) FilterEIP712DomainChanged(opts *bind.FilterOpts) (*RegistryEIP712DomainChangedIterator, error) {

	logs, sub, err := _Registry.contract.FilterLogs(opts, "EIP712DomainChanged")
	if err != nil {
		return nil, err
	}
	return &RegistryEIP712DomainChangedIterator{contract: _Registry.contract, event: "EIP712DomainChanged", logs: logs, sub: sub}, nil
}

// WatchEIP712DomainChanged is a free log subscription operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_Registry *RegistryFilterer) WatchEIP712DomainChanged(opts *bind.WatchOpts, sink chan<- *RegistryEIP712DomainChanged) (event.Subscription, error) {

	logs, sub, err := _Registry.contract.WatchLogs(opts, "EIP712DomainChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RegistryEIP712DomainChanged)
				if err := _Registry.contract.UnpackLog(event, "EIP712DomainChanged", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseEIP712DomainChanged is a log parse operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_Registry *RegistryFilterer) ParseEIP712DomainChanged(log types.Log) (*RegistryEIP712DomainChanged, error) {
	event := new(RegistryEIP712DomainChanged)
	if err := _Registry.contract.UnpackLog(event, "EIP712DomainChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// RegistryRegisteredIterator is returned from FilterRegistered and is used to iterate over the raw logs and unpacked data for Registered events raised by the Registry contract.
type RegistryRegisteredIterator struct {
	Event *RegistryRegistered // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *RegistryRegisteredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RegistryRegistered)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(RegistryRegistered)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *RegistryRegisteredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RegistryRegisteredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RegistryRegistered represents a Registered event raised by the Registry contract.
type RegistryRegistered struct {
	Key         [32]byte
	Signer      common.Address
	Registrant  common.Address
	Platform    string
	Service     string
	Version     string
	Measurement [32]byte
	Expires     uint64
	Signature   []byte
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterRegistered is a free log retrieval operation binding the contract event 0xba44e98c24a9fecda812bd8a5b3525cb7f075f8de749aea2440e34b3573b5004.
//
// Solidity: event Registered(bytes32 indexed key, address indexed signer, address registrant, string platform, string service, string version, bytes32 measurement, uint64 expires, bytes signature)
func (_Registry *RegistryFilterer) FilterRegistered(opts *bind.FilterOpts, key [][32]byte, signer []common.Address) (*RegistryRegisteredIterator, error) {

	var keyRule []interface{}
	for _, keyItem := range key {
		keyRule = append(keyRule, keyItem)
	}
	var signerRule []interface{}
	for _, signerItem := range signer {
		signerRule = append(signerRule, signerItem)
	}

	logs, sub, err := _Registry.contract.FilterLogs(opts, "Registered", keyRule, signerRule)
	if err != nil {
		return nil, err
	}
	return &RegistryRegisteredIterator{contract: _Registry.contract, event: "Registered", logs: logs, sub: sub}, nil
}

// WatchRegistered is a free log subscription operation binding the contract event 0xba44e98c24a9fecda812bd8a5b3525cb7f075f8de749aea2440e34b3573b5004.
//
// Solidity: event Registered(bytes32 indexed key, address indexed signer, address registrant, string platform, string service, string version, bytes32 measurement, uint64 expires, bytes signature)
func (_Registry *RegistryFilterer) WatchRegistered(opts *bind.WatchOpts, sink chan<- *RegistryRegistered, key [][32]byte, signer []common.Address) (event.Subscription, error) {

	var keyRule []interface{}
	for _, keyItem := range key {
		keyRule = append(keyRule, keyItem)
	}
	var signerRule []interface{}
	for _, signerItem := range signer {
		signerRule = append(signerRule, signerItem)
	}

	logs, sub, err := _Registry.contract.WatchLogs(opts, "Registered", keyRule, signerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RegistryRegistered)
				if err := _Registry.contract.UnpackLog(event, "Registered", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRegistered is a log parse operation binding the contract event 0xba44e98c24a9fecda812bd8a5b3525cb7f075f8de749aea2440e34b3573b5004.
//
// Solidity: event Registered(bytes32 indexed key, address indexed signer, address registrant, string platform, string service, string version, bytes32 measurement, uint64 expires, bytes signature)
func (_Registry *RegistryFilterer) ParseRegistered(log types.Log) (*RegistryRegistered, error) {
	event := new(RegistryRegistered)
	if err := _Registry.contract.UnpackLog(event, "Registered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
