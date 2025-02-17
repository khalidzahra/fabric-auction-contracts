package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type EnergyResource struct {
	Volume        float64 `json:"volume"`
	Price         float64 `json:"price"`
	Type          string  `json:"type"`
	IsAvailable   bool    `json:"isAvailable"`
	AuctionStatus bool    `json:"auctionStatus"`
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
		Volume:        energyVolume,
		Price:         energyPrice,
		Type:          resourceType,
		IsAvailable:   true,
		AuctionStatus: false,
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

func (ac *EnergyAuctionContract) GetMeritOrder(ctx contractapi.TransactionContextInterface) ([]EnergyResource, error) {
	results, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer results.Close()

	var resources []EnergyResource
	for results.HasNext() {
		next, err := results.Next()
		if err != nil {
			return nil, err
		}

		var resource EnergyResource
		err = json.Unmarshal(next.Value, &resource)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}

	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Price < resources[j].Price
	})

	return resources, nil
}

func (ac *EnergyAuctionContract) StartAuction(ctx contractapi.TransactionContextInterface, resourceID string, duration int64) error {
	fetchedResource, err := ctx.GetStub().GetState(resourceID)

	if err != nil {
		return fmt.Errorf("failed to retrieve resource: %v", err)
	}

	if fetchedResource == nil {
		return fmt.Errorf("resource with ID %s does not exist", resourceID)
	}

	var resource EnergyResource
	err = json.Unmarshal(fetchedResource, &resource)
	if err != nil {
		return fmt.Errorf("failed to unmarshal resource: %v", err)
	}

	if resource.AuctionStatus {
		return fmt.Errorf("auction for resource with ID %s is already active", resourceID)
	}

	if !resource.IsAvailable {
		return fmt.Errorf("resource with ID %s is not available", resourceID)
	}

	currentTimeStamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	auction := EnergyAuction{
		ResourceID:    resourceID,
		Deadline:      currentTimeStamp.Seconds + duration,
		HighestBid:    0,
		HighestBidder: "",
		IsActive:      true,
	}

	resource.AuctionStatus = true
	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal resource: %v", err)
	}

	err = ctx.GetStub().PutState(resourceID, resourceJSON)
	if err != nil {
		return fmt.Errorf("failed to update resource: %v", err)
	}

	auctionJSON, err := json.Marshal(auction)
	if err != nil {
		return fmt.Errorf("failed to marshal auction: %v", err)
	}

	return ctx.GetStub().PutState("auction:"+resourceID, auctionJSON)
}

func (ac *EnergyAuctionContract) GetAuction(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	auctionID := "auction:" + resourceID

	fetchedAuction, err := ctx.GetStub().GetState(auctionID)

	if err != nil {
		return "", fmt.Errorf("failed to retrieve auction: %v", err)
	}

	if fetchedAuction == nil {
		return "", fmt.Errorf("auction with ID %s does not exist", auctionID)
	}

	return string(fetchedAuction), nil
}

func (ac *EnergyAuctionContract) Bid(ctx contractapi.TransactionContextInterface, resourceID string, bidAmount float64) error {
	auctionID := "auction:" + resourceID

	fetchedAuction, err := ctx.GetStub().GetState(auctionID)

	if err != nil {
		return fmt.Errorf("failed to retrieve auction: %v", err)
	}

	if fetchedAuction == nil {
		return fmt.Errorf("auction with ID %s does not exist", auctionID)
	}

	fetchedResource, err := ctx.GetStub().GetState(resourceID)

	if err != nil {
		return fmt.Errorf("failed to retrieve resource: %v", err)
	}

	if fetchedResource == nil {
		return fmt.Errorf("resource with ID %s does not exist", resourceID)
	}

	var resource EnergyResource
	err = json.Unmarshal(fetchedResource, &resource)
	if err != nil {
		return fmt.Errorf("failed to unmarshal resource: %v", err)
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
		return ac.EndAuction(ctx, resourceID)
	}

	if bidAmount <= resource.Price {
		return fmt.Errorf("bid amount must be higher than resource price")
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

func (ac *EnergyAuctionContract) EndAuction(ctx contractapi.TransactionContextInterface, resourceID string) error {
	auctionID := "auction:" + resourceID

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

	winner := auction.HighestBidder
	winningBid := auction.HighestBid
	fmt.Printf("auction has been ended. Winner: %s with a bid of: %f\n", winner, winningBid)

	auction.IsActive = false
	auctionJSON, err := json.Marshal(auction)
	if err != nil {
		return fmt.Errorf("failed to marshal auction: %v", err)
	}

	err = ctx.GetStub().PutState(auctionID, auctionJSON)
	if err != nil {
		return fmt.Errorf("failed to update auction: %v", err)
	}

	fetchedResource, err := ctx.GetStub().GetState(auction.ResourceID)

	if err != nil {
		return fmt.Errorf("failed to retrieve resource: %v", err)
	}

	if fetchedResource == nil {
		return fmt.Errorf("resource with ID %s does not exist", auction.ResourceID)
	}

	var resource EnergyResource
	err = json.Unmarshal(fetchedResource, &resource)
	if err != nil {
		return fmt.Errorf("failed to unmarshal resource: %v", err)
	}

	resource.AuctionStatus = false

	if auction.HighestBidder != "" {
		resource.IsAvailable = false
	}

	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal resource: %v", err)
	}

	err = ctx.GetStub().PutState(auction.ResourceID, resourceJSON)
	if err != nil {
		return fmt.Errorf("failed to update resource: %v", err)
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
