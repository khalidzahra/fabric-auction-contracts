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

	return ac.storeResource(ctx, resourceID, resource)
}

func (ac *EnergyAuctionContract) GetResource(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	fetchedResource, err := ac.fetchAndUnmarshal(ctx, resourceID, "resource")
	if err != nil {
		return "", err
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
	ac.storeResource(ctx, resourceID, *resource)

	return ac.storeAuction(ctx, "auction:"+resourceID, auction)
}

func (ac *EnergyAuctionContract) GetAuction(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	auctionID := "auction:" + resourceID

	fetchedAuction, err := ac.fetchAndUnmarshal(ctx, auctionID, "auction")
	if err != nil {
		return "", err
	}

	return string(fetchedAuction), nil
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

	return ac.storeAuction(ctx, auctionID, *auction)
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
	ac.storeAuction(ctx, auctionID, *auction)

	resource, err := ac.fetchResource(ctx, resourceID)
	if err != nil {
		return err
	}

	resource.AuctionStatus = false

	if auction.HighestBidder != "" {
		resource.IsAvailable = false
	}

	ac.storeResource(ctx, resourceID, *resource)

	return ac.storeAuction(ctx, auctionID, *auction)
}

// Helper functions
func (ac *EnergyAuctionContract) fetchAndUnmarshal(ctx contractapi.TransactionContextInterface, key, item string) ([]byte, error) {
	fetchedState, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve %s: %v", item, err)
	}
	if fetchedState == nil {
		return nil, fmt.Errorf("%s with ID %s does not exist", item, key)
	}
	return fetchedState, nil
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

func (ac *EnergyAuctionContract) storeResource(ctx contractapi.TransactionContextInterface, resourceID string, resource EnergyResource) error {
	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal resource: %v", err)
	}
	return ctx.GetStub().PutState(resourceID, resourceJSON)
}

func (ac *EnergyAuctionContract) storeAuction(ctx contractapi.TransactionContextInterface, auctionID string, auction EnergyAuction) error {
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
