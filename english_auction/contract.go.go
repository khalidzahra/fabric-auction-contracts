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
	if err := ac.checkResourceExists(ctx, resourceID); err != nil {
		return err
	}

	resource := EnergyResource{
		Volume:        energyVolume,
		Price:         energyPrice,
		Type:          resourceType,
		IsAvailable:   true,
		AuctionStatus: false,
	}

	return ac.storeObject(ctx, resourceID, resource)
}

func (ac *EnergyAuctionContract) GetResource(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	resource, err := ac.fetchResource(ctx, resourceID)
	if err != nil {
		return "", err
	}
	return ac.marshalToString(resource)
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
		if err != nil { // Not an EnergyResource object
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
	resource, err := ac.fetchResource(ctx, resourceID)

	if err != nil {
		return err
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
	ac.storeObject(ctx, resourceID, *resource)

	return ac.storeObject(ctx, "auction:"+resourceID, auction)
}

func (ac *EnergyAuctionContract) GetAuction(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	auction, err := ac.fetchAuction(ctx, "auction:"+resourceID)

	if err != nil {
		return "", err
	}

	return ac.marshalToString(auction)
}

func (ac *EnergyAuctionContract) Bid(ctx contractapi.TransactionContextInterface, resourceID string, bidAmount float64) error {
	auctionID := "auction:" + resourceID

	resource, err := ac.fetchResource(ctx, resourceID)
	if err != nil {
		return err
	}

	auction, err := ac.fetchAuction(ctx, auctionID)
	if err != nil {
		return err
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

	return ac.storeObject(ctx, auctionID, *auction)
}

func (ac *EnergyAuctionContract) EndAuction(ctx contractapi.TransactionContextInterface, resourceID string) error {
	auctionID := "auction:" + resourceID

	auction, err := ac.fetchAuction(ctx, auctionID)
	if err != nil {
		return err
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
	ac.storeObject(ctx, auctionID, *auction)

	resource, err := ac.fetchResource(ctx, resourceID)
	if err != nil {
		return err
	}

	resource.AuctionStatus = false

	if auction.HighestBidder != "" {
		resource.IsAvailable = false
	}

	ac.storeObject(ctx, resourceID, *resource)

	return ac.storeObject(ctx, auctionID, *auction)
}

// Helper functions
func (ac *EnergyAuctionContract) marshalToString(v interface{}) (string, error) {
	jsonData, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to marshal: %v", err)
	}
	return string(jsonData), nil
}

func (ac *EnergyAuctionContract) checkResourceExists(ctx contractapi.TransactionContextInterface, resourceID string) error {
	fetchedResource, err := ctx.GetStub().GetState(resourceID)
	if err != nil {
		return fmt.Errorf("failed to interact with world state: %v", err)
	}
	if fetchedResource != nil {
		return fmt.Errorf("a resource already exists with ID: %s", resourceID)
	}
	return nil
}

func (ac *EnergyAuctionContract) fetchResource(ctx contractapi.TransactionContextInterface, resourceID string) (*EnergyResource, error) {
	fetchedResource, err := ctx.GetStub().GetState(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve resource: %v", err)
	}
	if fetchedResource == nil {
		return nil, fmt.Errorf("resource with ID %s does not exist", resourceID)
	}

	var resource EnergyResource
	if err := json.Unmarshal(fetchedResource, &resource); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource: %v", err)
	}
	return &resource, nil
}

func (ac *EnergyAuctionContract) fetchAuction(ctx contractapi.TransactionContextInterface, auctionID string) (*EnergyAuction, error) {
	fetchedAuction, err := ctx.GetStub().GetState(auctionID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve auction: %v", err)
	}
	if fetchedAuction == nil {
		return nil, fmt.Errorf("auction with ID %s does not exist", auctionID)
	}

	var auction EnergyAuction
	if err := json.Unmarshal(fetchedAuction, &auction); err != nil {
		return nil, fmt.Errorf("failed to unmarshal auction: %v", err)
	}
	return &auction, nil
}

func (ac *EnergyAuctionContract) storeObject(ctx contractapi.TransactionContextInterface, key string, object interface{}) error {
	objectJSON, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %v", err)
	}
	return ctx.GetStub().PutState(key, objectJSON)
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
