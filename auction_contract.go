package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type EnergyResource struct {
	Volume float64 `json:"volume"`
	Price  float64 `json:"price"`
	Type   string  `json:"type"`
}

type EnergyAuction struct {
	AuctionID     string  `json:"auctionID"`
	ResourceID    string  `json:"resourceID"`
	Deadline      int64   `json:"deadline"`
	HighestBid    float64 `json:"highestBid"`
	HighestBidder string  `json:"highestBidder"`
	IsActive      bool    `json:"status"`
}

type EnergyAuctionContract struct {
	contractapi.Contract
}

func (ac *EnergyAuctionContract) SubmitEnergyResource(ctx contractapi.TransactionContextInterface, resourceID string, energyVolume, energyPrice float64, resourceType string) error {
	fetchedResource, err := ctx.GetStub().GetState(resourceID)

	if err != nil {
		return fmt.Errorf("failed to interact with world state: %v", err)
	}

	if fetchedResource != nil {
		return fmt.Errorf("a resource already exists with ID: %s", resourceID)
	}

	resource := EnergyResource{
		Volume: energyVolume,
		Price:  energyPrice,
		Type:   resourceType,
	}

	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal resource: %v", err)
	}

	return ctx.GetStub().PutState(resourceID, resourceJSON)
}

func (ac *EnergyAuctionContract) GetResource(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	fetchedResource, err := ctx.GetStub().GetState(resourceID)

	if err != nil {
		return "", fmt.Errorf("failed to retrieve resource: %v", err)
	}

	if fetchedResource == nil {
		return "", fmt.Errorf("resource with ID %s does not exist", resourceID)
	}

	return string(fetchedResource), nil
}

func (ac *EnergyAuctionContract) StartAuction(ctx contractapi.TransactionContextInterface, auctionID, resourceID string, duration int64) error {
	fetchedResource, err := ctx.GetStub().GetState(resourceID)

	if err != nil {
		return fmt.Errorf("failed to retrieve resource: %v", err)
	}

	if fetchedResource == nil {
		return fmt.Errorf("resource with ID %s does not exist", resourceID)
	}

	currentTimeStamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	auction := EnergyAuction{
		AuctionID:     auctionID,
		ResourceID:    resourceID,
		Deadline:      currentTimeStamp.Seconds + duration,
		HighestBid:    0,
		HighestBidder: "",
		IsActive:      true,
	}

	auctionJSON, err := json.Marshal(auction)
	if err != nil {
		return fmt.Errorf("failed to marshal auction: %v", err)
	}

	return ctx.GetStub().PutState(auctionID, auctionJSON)
}

func (ac *EnergyAuctionContract) GetAuction(ctx contractapi.TransactionContextInterface, auctionID string) (string, error) {
	fetchedAuction, err := ctx.GetStub().GetState(auctionID)

	if err != nil {
		return "", fmt.Errorf("failed to retrieve auction: %v", err)
	}

	if fetchedAuction == nil {
		return "", fmt.Errorf("auction with ID %s does not exist", auctionID)
	}

	return string(fetchedAuction), nil
}

func (ac *EnergyAuctionContract) Bid(ctx contractapi.TransactionContextInterface, auctionID string, bidAmount float64) error {
	fetchedAuction, err := ctx.GetStub().GetState(auctionID)

	if err != nil {
		return fmt.Errorf("failed to retrieve auction: %v", err)
	}

	if fetchedAuction == nil {
		return fmt.Errorf("auction with ID %s does not exist", auctionID)
	}

	var auction EnergyAuction
	err = json.Unmarshal(fetchedAuction, &auction)
	if err != nil {
		return fmt.Errorf("failed to unmarshal auction: %v", err)
	}

	if !auction.IsActive {
		return fmt.Errorf("auction with ID %s is not active", auctionID)
	}

	currentTimeStamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	if auction.Deadline < currentTimeStamp.Seconds {
		return fmt.Errorf("auction with ID %s has expired", auctionID)
	}

	if bidAmount <= auction.HighestBid {
		return fmt.Errorf("bid amount must be higher than current highest bid")
	}

	clientId, err := ctx.GetClientIdentity().GetID()

	if err != nil {
		return fmt.Errorf("failed to get client ID: %v", err)
	}

	auction.HighestBid = bidAmount
	auction.HighestBidder = clientId

	auctionJSON, err := json.Marshal(auction)
	if err != nil {
		return fmt.Errorf("failed to marshal auction: %v", err)
	}

	return ctx.GetStub().PutState(auctionID, auctionJSON)
}

func (ac *EnergyAuctionContract) EndAuction(ctx contractapi.TransactionContextInterface, auctionID string) error {
	fetchedAuction, err := ctx.GetStub().GetState(auctionID)

	if err != nil {
		return fmt.Errorf("failed to retrieve auction: %v", err)
	}

	if fetchedAuction == nil {
		return fmt.Errorf("auction with ID %s does not exist", auctionID)
	}

	var auction EnergyAuction
	err = json.Unmarshal(fetchedAuction, &auction)
	if err != nil {
		return fmt.Errorf("failed to unmarshal auction: %v", err)
	}

	if !auction.IsActive {
		return fmt.Errorf("auction with ID %s is not active", auctionID)
	}

	currentTimeStamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	if auction.Deadline > currentTimeStamp.Seconds {
		return fmt.Errorf("auction with ID %s has not yet expired", auctionID)
	}

	if auction.HighestBidder == "" {
		return fmt.Errorf("no bids were placed for auction with ID %s", auctionID)
	}

	winner := auction.HighestBidder
	winningBid := auction.HighestBid
	fmt.Printf("auction has been ended. Winner: %s with a bid of: %f\n", winner, winningBid)

	auction.IsActive = false
	auctionJSON, err := json.Marshal(auction)
	if err != nil {
		return fmt.Errorf("failed to marshal auction: %v", err)
	}

	return ctx.GetStub().PutState(auctionID, auctionJSON)
}

func main() {
	chaincode, err := contractapi.NewChaincode(&EnergyAuctionContract{})
	if err != nil {
		log.Panicf("Error creating asset chaincode: %v", err)
	}

	if err := chaincode.Start(); err != nil {
		log.Panicf("Error starting asset chaincode: %v", err)
	}
}
