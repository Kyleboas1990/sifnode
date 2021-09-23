package txs

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	abci "github.com/tendermint/tendermint/abci/types"
	"go.uber.org/zap"

	"github.com/Sifchain/sifnode/cmd/ebrelayer/internal/symbol_translator"
	"github.com/Sifchain/sifnode/cmd/ebrelayer/types"
	ethbridge "github.com/Sifchain/sifnode/x/ethbridge/types"
	oracletypes "github.com/Sifchain/sifnode/x/oracle/types"
)

const (
	nullAddress = "0x0000000000000000000000000000000000000000"
)

// EthereumEventToEthBridgeClaim parses and packages an Ethereum event struct with a validator address in an EthBridgeClaim msg
func EthereumEventToEthBridgeClaim(valAddr sdk.ValAddress, event types.EthereumEvent, symbolTranslator *symbol_translator.SymbolTranslator, sugaredLogger *zap.SugaredLogger) (ethbridge.EthBridgeClaim, error) {
	witnessClaim := ethbridge.EthBridgeClaim{}

	// chainID type casting (*big.Int -> int)
	networkDescriptor := event.NetworkDescriptor

	bridgeContractAddress := ethbridge.NewEthereumAddress(event.BridgeContractAddress.Hex())

	// Sender type casting (address.common -> string)
	sender := ethbridge.NewEthereumAddress(event.From.Hex())

	// Recipient type casting ([]bytes -> sdk.AccAddress)
	recipient, err := sdk.AccAddressFromBech32(string(event.To))
	if err != nil {
		return witnessClaim, err
	}
	if recipient.Empty() {
		return witnessClaim, errors.New("empty recipient address")
	}

	// Sender type casting (address.common -> string)
	tokenContractAddress := ethbridge.NewEthereumAddress(event.Token.Hex())

	// Symbol formatted to lowercase
	symbol := strings.ToLower(event.Symbol)
	switch event.ClaimType {
	case ethbridge.ClaimType_CLAIM_TYPE_LOCK:
		if symbol == "eth" && !isZeroAddress(event.Token) {
			return witnessClaim, errors.New("symbol \"eth\" must have null address set as token address")
		}
	case ethbridge.ClaimType_CLAIM_TYPE_BURN:
		symbol = symbolTranslator.EthereumToSifchain(symbol)
	}

	amount := sdk.NewIntFromBigInt(event.Value)

	// Nonce type casting (*big.Int -> int)
	nonce := int(event.Nonce.Int64())

	// Package the information in a unique EthBridgeClaim
	witnessClaim.NetworkDescriptor = networkDescriptor
	witnessClaim.BridgeContractAddress = bridgeContractAddress.String()
	witnessClaim.Nonce = int64(nonce)
	witnessClaim.TokenContractAddress = tokenContractAddress.String()
	witnessClaim.Symbol = symbol
	witnessClaim.EthereumSender = sender.String()
	witnessClaim.ValidatorAddress = valAddr.String()
	witnessClaim.CosmosReceiver = recipient.String()
	witnessClaim.Amount = amount
	witnessClaim.ClaimType = event.ClaimType
	witnessClaim.Decimals = int64(event.Decimals)
	witnessClaim.TokenName = event.Name
	witnessClaim.DenomHash = ethbridge.GetDenomHash(networkDescriptor, tokenContractAddress.String(), int64(event.Decimals), event.Name, event.Symbol)

	return witnessClaim, nil
}

// BurnLockEventToCosmosMsg parses data from a Burn/Lock event witnessed on Cosmos into a CosmosMsg struct
func BurnLockEventToCosmosMsg(attributes []abci.EventAttribute, sugaredLogger *zap.SugaredLogger) (types.CosmosMsg, error) {
	var prophecyID []byte
	var networkDescriptor uint32

	attributeNumber := 0

	for _, attribute := range attributes {
		key := string(attribute.GetKey())
		val := string(attribute.GetValue())

		fmt.Printf(" key is %v, value is %v\n", key, val)

		// Set variable based on the attribute's key
		switch key {
		case types.ProphecyID.String():
			fmt.Printf(" prophecy id is %v\n", val)
			prophecyID = []byte(val)
			attributeNumber++

		case types.NetworkDescriptor.String():
			attributeNumber++
			tempNetworkDescriptor, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				sugaredLogger.Errorw("network id can't parse", "networkDescriptor", val)
				return types.CosmosMsg{}, errors.New("network id can't parse")
			}
			networkDescriptor = uint32(tempNetworkDescriptor)

			// check if the networkDescriptor is valid
			if !oracletypes.NetworkDescriptor(networkDescriptor).IsValid() {
				return types.CosmosMsg{}, errors.New("network id is invalid")
			}
		}
	}

	if attributeNumber < 2 {
		sugaredLogger.Errorw("message not complete", "attributeNumber", attributeNumber)
		return types.CosmosMsg{}, errors.New("message not complete")
	}
	return types.NewCosmosMsg(oracletypes.NetworkDescriptor(networkDescriptor), prophecyID), nil
}

// AttributesToEthereumBridgeClaim parses data from event to EthereumBridgeClaim
func AttributesToEthereumBridgeClaim(attributes []abci.EventAttribute) (types.EthereumBridgeClaim, error) {
	var cosmosSender sdk.ValAddress
	var ethereumSenderNonce sdk.Int
	var ethereumSender common.Address
	var err error

	for _, attribute := range attributes {
		key := string(attribute.GetKey())
		val := string(attribute.GetValue())

		// Set variable based on the attribute's key
		switch key {
		case types.CosmosSender.String():
			cosmosSender, err = sdk.ValAddressFromBech32(val)
			if err != nil {
				return types.EthereumBridgeClaim{}, err
			}

		case types.EthereumSender.String():
			if !common.IsHexAddress(val) {
				log.Printf("Invalid recipient address: %v", val)
				return types.EthereumBridgeClaim{}, errors.New("invalid recipient address: " + val)
			}
			ethereumSender = common.HexToAddress(val)

		case types.EthereumSenderNonce.String():
			tempNonce, ok := sdk.NewIntFromString(val)
			if !ok {
				log.Println("Invalid nonce:", val)
				return types.EthereumBridgeClaim{}, errors.New("invalid nonce:" + val)
			}
			ethereumSenderNonce = tempNonce
		}
	}

	return types.EthereumBridgeClaim{
		EthereumSender: ethereumSender,
		CosmosSender:   cosmosSender,
		Nonce:          ethereumSenderNonce,
	}, nil
}

// AttributesToCosmosSignProphecyClaim parses data from event to EthereumBridgeClaim
func AttributesToCosmosSignProphecyClaim(attributes []abci.EventAttribute) (types.CosmosSignProphecyClaim, error) {
	var cosmosSender sdk.ValAddress
	var networkDescriptor oracletypes.NetworkDescriptor
	var prophecyID []byte
	var err error
	attributeNumber := 0

	for _, attribute := range attributes {
		key := string(attribute.GetKey())
		val := string(attribute.GetValue())

		// Set variable based on the attribute's key
		switch key {
		case types.CosmosSender.String():
			cosmosSender, err = sdk.ValAddressFromBech32(val)
			if err != nil {
				return types.CosmosSignProphecyClaim{}, err
			}

		case types.NetworkDescriptor.String():
			attributeNumber++
			tempNetworkDescriptor, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				log.Printf("network id can't parse networkDescriptor is %s\n", val)
				return types.CosmosSignProphecyClaim{}, errors.New("network id can't parse")
			}
			networkDescriptor = oracletypes.NetworkDescriptor(uint32(tempNetworkDescriptor))

			// check if the networkDescriptor is valid
			if !networkDescriptor.IsValid() {
				return types.CosmosSignProphecyClaim{}, errors.New("network id is invalid")
			}

		case types.ProphecyID.String():
			prophecyID = []byte(val)
		}
	}

	return types.CosmosSignProphecyClaim{
		CosmosSender:      cosmosSender,
		NetworkDescriptor: networkDescriptor,
		ProphecyID:        prophecyID,
	}, nil
}

// isZeroAddress checks an Ethereum address and returns a bool which indicates if it is the null address
func isZeroAddress(address common.Address) bool {
	return address == common.HexToAddress(nullAddress)
}
