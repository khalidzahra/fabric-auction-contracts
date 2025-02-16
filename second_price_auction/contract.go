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
	AuctionID   string  `json:"auctionID"`
	ResourceID  string  `json:"resourceID"`
	Deadline    int64   `json:"deadline"`
	Bids        []Bid   `json:"bids"`
	WinnerID    string  `json:"winnerID"`
	WinnerPrice float64 `json:"winnerPrice"`
	IsActive    bool    `json:"status"`
}

type Bid struct {
	BidID      string  `json:"bidID"`
	ResourceID string  `json:"resourceID"`
	Bidder     string  `json:"bidder"`
	BidPrice   float64 `json:"bidPrice"`
	Timestamp  int64   `json:"timestamp"`
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
		ResourceID: resourceID,
		Deadline:   currentTimeStamp.Seconds + duration,
		Bids:       []Bid{},
		IsActive:   true,
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

	var auction EnergyAuction
	err = json.Unmarshal(fetchedAuction, &auction)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal auction %s: %v", auctionID, err)
	}

	currentTimeStamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	if auction.Deadline > currentTimeStamp.Seconds {
		hiddenAuction := EnergyAuction{
			AuctionID:   auction.AuctionID,
			ResourceID:  auction.ResourceID,
			Deadline:    auction.Deadline,
			Bids:        []Bid{},
			WinnerID:    "",
			WinnerPrice: 0,
			IsActive:    auction.IsActive,
		}

		hiddenAuctionJSON, err := json.Marshal(hiddenAuction)
		if err != nil {
			return "", fmt.Errorf("failed to marshal auction: %v", err)
		}

		return string(hiddenAuctionJSON), nil
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

	clientId, err := ctx.GetClientIdentity().GetID()

	if err != nil {
		return fmt.Errorf("failed to get client ID: %v", err)
	}

	bid := Bid{
		BidID:      auctionID + ":" + clientId + ":" + fmt.Sprint(currentTimeStamp.Seconds),
		ResourceID: resourceID,
		Bidder:     clientId,
		BidPrice:   bidAmount,
		Timestamp:  currentTimeStamp.Seconds,
	}

	auction.Bids = append(auction.Bids, bid)

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

	sort.Slice(auction.Bids, func(i, j int) bool {
		return auction.Bids[i].BidPrice > auction.Bids[j].BidPrice
	})

	auction.IsActive = false

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

	if len(auction.Bids) > 0 {
		resource.IsAvailable = false

		auction.WinnerID = auction.Bids[0].Bidder
		if len(auction.Bids) > 1 {
			auction.WinnerPrice = auction.Bids[1].BidPrice
		} else {
			auction.WinnerPrice = auction.Bids[0].BidPrice
		}
	}

	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal resource: %v", err)
	}

	err = ctx.GetStub().PutState(auction.ResourceID, resourceJSON)
	if err != nil {
		return fmt.Errorf("failed to update resource: %v", err)
	}

	auctionJSON, err := json.Marshal(auction)
	if err != nil {
		return fmt.Errorf("failed to marshal auction: %v", err)
	}

	err = ctx.GetStub().PutState(auctionID, auctionJSON)
	if err != nil {
		return fmt.Errorf("failed to update auction: %v", err)
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
