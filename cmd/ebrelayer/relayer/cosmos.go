package relayer

// DONTCOVER

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Sifchain/sifnode/cmd/ebrelayer/internal/symbol_translator"
	"google.golang.org/grpc"

	"github.com/Sifchain/sifnode/cmd/ebrelayer/txs"
	"github.com/Sifchain/sifnode/cmd/ebrelayer/types"
	ethbridgetypes "github.com/Sifchain/sifnode/x/ethbridge/types"
	oracletypes "github.com/Sifchain/sifnode/x/oracle/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	tmclient "github.com/tendermint/tendermint/rpc/client/http"

	"go.uber.org/zap"
)

// TODO: Move relay functionality out of CosmosSub into a new Relayer parent struct
const errorMessageKey = "errorMessage"

// CosmosSub defines a Cosmos listener that relays events to Ethereum and Cosmos
type CosmosSub struct {
	TmProvider              string
	EthProvider             string
	PrivateKey              *ecdsa.PrivateKey
	SugaredLogger           *zap.SugaredLogger
	NetworkDescriptor       oracletypes.NetworkDescriptor
	RegistryContractAddress common.Address
	CliContext              client.Context
	ValidatorName           string
}

// NewCosmosSub initializes a new CosmosSub
func NewCosmosSub(networkDescriptor oracletypes.NetworkDescriptor, privateKey *ecdsa.PrivateKey, tmProvider, ethProvider string, registryContractAddress common.Address,
	cliContext client.Context, validatorName string, sugaredLogger *zap.SugaredLogger) CosmosSub {

	return CosmosSub{
		NetworkDescriptor:       networkDescriptor,
		TmProvider:              tmProvider,
		PrivateKey:              privateKey,
		EthProvider:             ethProvider,
		RegistryContractAddress: registryContractAddress,
		CliContext:              cliContext,
		ValidatorName:           validatorName,
		SugaredLogger:           sugaredLogger,
	}
}

// Start a Cosmos chain subscription
func (sub CosmosSub) Start(txFactory tx.Factory, completionEvent *sync.WaitGroup, symbolTranslator *symbol_translator.SymbolTranslator) {
	defer completionEvent.Done()
	time.Sleep(time.Second)
	client, err := tmclient.New(sub.TmProvider, "/websocket")
	if err != nil {
		sub.SugaredLogger.Errorw("failed to initialize a sifchain client.",
			errorMessageKey, err.Error())
		completionEvent.Add(1)
		go sub.Start(txFactory, completionEvent, symbolTranslator)
		return
	}

	if err := client.Start(); err != nil {
		sub.SugaredLogger.Errorw("failed to start a sifchain client.",
			errorMessageKey, err.Error())
		completionEvent.Add(1)
		go sub.Start(txFactory, completionEvent, symbolTranslator)
		return
	}

	defer client.Stop() //nolint:errcheck

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer close(quit)

	// start the timer
	t := time.NewTicker(time.Second * ethereumWakeupTimer)
	for {
		select {
		// Handle any errors
		case <-quit:
			log.Println("we receive the quit signal and exit")
			return
		case <-t.C:
			sub.CheckNonceAndProcess(txFactory, client)
		}
	}

}

// CheckNonceAndProcess check the lock burn nonce and process the event
func (sub CosmosSub) CheckNonceAndProcess(txFactory tx.Factory,
	tmclient *tmclient.HTTP) {

	// get lock burn nonce from cosmos
	globalNonce, err := sub.GetWitnessLockBurnNonceFromCosmos(oracletypes.NetworkDescriptor(networkID.Uint64()), string(sub.ValidatorAddress))
	if err != nil {
		sub.SugaredLogger.Errorw("failed to get the lock burn nonce from cosmos rpc",
			errorMessageKey, err.Error())
		return
	}
}

func (sub CosmosSub) ProcessLockBurnWithScope(txFactory tx.Factory, client *tmclient.HTTP, fromBlockNumber int64, toBlockNumber int64) {
	for blockNumber := fromBlockNumber; blockNumber <= toBlockNumber; {
		tmpBlockNumber := blockNumber

		ctx := context.Background()
		block, err := client.BlockResults(ctx, &tmpBlockNumber)

		if err != nil {
			sub.SugaredLogger.Errorw("sifchain client failed to get a block.",
				errorMessageKey, err.Error())
			continue
		}

		for _, txLog := range block.TxsResults {
			sub.SugaredLogger.Infow("block.TxsResults: ", "block.TxsResults: ", block.TxsResults)
			for _, event := range txLog.Events {

				claimType := getOracleClaimType(event.GetType())

				sub.SugaredLogger.Infow("claimtype cosmos.go: ", "claimType: ", claimType)

				switch claimType {
				case types.MsgBurn, types.MsgLock:
					// the relayer for signature aggregator not handle burn and lock
					cosmosMsg, err := txs.BurnLockEventToCosmosMsg(event.GetAttributes(), sub.SugaredLogger)
					if err != nil {
						sub.SugaredLogger.Errorw("sifchain client failed in get burn lock message from event.",
							errorMessageKey, err.Error())
						continue
					}

					sub.SugaredLogger.Infow(
						"Received message from sifchain: ",
						"msg", cosmosMsg,
					)

					if cosmosMsg.NetworkDescriptor == sub.NetworkDescriptor {
						sub.witnessSignProphecyID(txFactory, cosmosMsg)
					}
				}
			}
		}

		blockNumber++
	}
}

// MessageProcessed check if cosmogs message already processed
func MessageProcessed(prophecyID []byte, prophecyClaims []types.ProphecyClaimUnique) bool {
	for _, prophecyClaim := range prophecyClaims {
		if bytes.Compare(prophecyID, prophecyClaim.ProphecyID) == 0 {

			return true
		}
	}
	return false
}

// getOracleClaimType sets the OracleClaim's claim type based upon the witnessed event type
func getOracleClaimType(eventType string) types.Event {
	var claimType types.Event
	switch eventType {
	case types.MsgBurn.String():
		claimType = types.MsgBurn
	case types.MsgLock.String():
		claimType = types.MsgLock
	case types.ProphecyCompleted.String():
		claimType = types.ProphecyCompleted
	default:
		claimType = types.Unsupported
	}
	return claimType
}

func tryInitRelayConfig(sub CosmosSub) (*ethclient.Client, *bind.TransactOpts, common.Address, error) {

	for i := 0; i < 5; i++ {
		client, auth, target, err := txs.InitRelayConfig(
			sub.EthProvider,
			sub.RegistryContractAddress,
			sub.PrivateKey,
			sub.SugaredLogger,
		)

		if err != nil {
			sub.SugaredLogger.Errorw("failed in init relay config.",
				errorMessageKey, err.Error())
			continue
		}
		return client, auth, target, err
	}

	return nil, nil, common.Address{}, errors.New("hit max initRelayConfig retries")
}

// witness node sign against prophecyID of lock and burn message and send the singnature in message back to Sifnode.
func (sub CosmosSub) witnessSignProphecyID(
	txFactory tx.Factory,
	cosmosMsg types.CosmosMsg,
) {
	sub.SugaredLogger.Infow("handle burn lock message.",
		"cosmosMessage", cosmosMsg.String())

	sub.SugaredLogger.Infow(
		"get the prophecy claim.",
		"cosmosMsg", cosmosMsg,
	)

	valAddr, err := GetValAddressFromKeyring(txFactory.Keybase(), sub.ValidatorName)
	if err != nil {
		sub.SugaredLogger.Infow(
			"get the prophecy claim.",
			"cosmosMsg", err,
		)
	}

	signData := txs.PrefixMsg(cosmosMsg.ProphecyID)
	address := crypto.PubkeyToAddress(sub.PrivateKey.PublicKey)
	signature, err := txs.SignClaim(signData, sub.PrivateKey)
	if err != nil {
		sub.SugaredLogger.Infow(
			"failed to sign the prophecy id",
			errorMessageKey, err.Error(),
		)
	}

	signProphecy := ethbridgetypes.NewMsgSignProphecy(valAddr.String(), cosmosMsg.NetworkDescriptor,
		cosmosMsg.ProphecyID, address.String(), string(signature))

	txs.SignProphecyToCosmos(txFactory, signProphecy, sub.CliContext, sub.SugaredLogger)
}

// GetWitnessLockBurnNonceFromCosmos get witness lock burn nonce via rpc
func (sub CosmosSub) GetWitnessLockBurnNonceFromCosmos(
	networkDescriptor oracletypes.NetworkDescriptor,
	relayerValAddress string) (uint64, error) {
	conn, err := grpc.Dial(sub.TmProvider)
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client := ethbridgetypes.NewQueryClient(conn)
	request := ethbridgetypes.QueryWitnessLockBurnNonceRequest{
		NetworkDescriptor: networkDescriptor,
		RelayerValAddress: relayerValAddress,
	}
	response, err := client.WitnessLockBurnNonce(ctx, &request)
	if err != nil {
		return 0, err
	}
	return response.WitnessLockBurnNonce, nil
}

// GetGlobalNonceBlockNumberFromCosmos get global nonce block number via rpc
func (sub CosmosSub) GetGlobalNonceBlockNumberFromCosmos(
	networkDescriptor oracletypes.NetworkDescriptor,
	relayerValAddress string) (uint64, uint64, error) {
	conn, err := grpc.Dial(sub.TmProvider)
	if err != nil {
		return 0, 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client := ethbridgetypes.NewQueryClient(conn)

	request := ethbridgetypes.QueryWitnessLockBurnNonceRequest{
		NetworkDescriptor: networkDescriptor,
		RelayerValAddress: relayerValAddress,
	}
	response, err := client.WitnessLockBurnNonce(ctx, &request)
	if err != nil {
		return 0, 0, err
	}
	globalNonce := response.WitnessLockBurnNonce

	request2 := ethbridgetypes.QueryGlocalNonceBlockNumberRequest{
		NetworkDescriptor: networkDescriptor,
		GlobalNonce:       globalNonce + 1,
	}

	response2, err := client.GlocalNonceBlockNumber(ctx, &request2)
	if err != nil {
		return 0, 0, err
	}

	return globalNonce, response2.BlockNumber, nil
}
