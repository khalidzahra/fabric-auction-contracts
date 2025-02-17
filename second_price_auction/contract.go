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

	currentTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	auction := EnergyAuction{
		ResourceID: resourceID,
		Deadline:   currentTimestamp.Seconds + duration,
		Bids:       []Bid{},
		IsActive:   true,
	}
	resource.AuctionStatus = true

	if err := ac.storeResource(ctx, resourceID, *resource); err != nil {
		return err
	}

	return ac.storeAuction(ctx, "auction:"+resourceID, auction)
}

func (ac *EnergyAuctionContract) GetAuction(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	auctionID := "auction:" + resourceID
	auction, err := ac.fetchAuction(ctx, auctionID)
	if err != nil {
		return "", err
	}

	currentTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	if auction.Deadline > currentTimestamp.Seconds {
		auction.Bids = []Bid{}
	}

	return ac.marshalToString(auction)
}

func (ac *EnergyAuctionContract) Bid(ctx contractapi.TransactionContextInterface, resourceID string, bidAmount float64) error {
	auctionID := "auction:" + resourceID
	auction, err := ac.fetchAuction(ctx, auctionID)
	if err != nil {
		return err
	}

	resource, err := ac.fetchResource(ctx, resourceID)
	if err != nil {
		return err
	}

	if bidAmount <= resource.Price {
		return fmt.Errorf("bid amount must be higher than resource price")
	}

	if !auction.IsActive {
		return fmt.Errorf("auction with ID %s is not active", auctionID)
	}

	currentTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	if auction.Deadline < currentTimestamp.Seconds {
		return ac.EndAuction(ctx, resourceID)
	}

	clientID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get client ID: %v", err)
	}

	bid := Bid{
		BidID:      fmt.Sprintf("%s:%s:%d", auctionID, clientID, currentTimestamp.Seconds),
		ResourceID: resourceID,
		Bidder:     clientID,
		BidPrice:   bidAmount,
		Timestamp:  currentTimestamp.Seconds,
	}

	auction.Bids = append(auction.Bids, bid)

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

	currentTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	if auction.Deadline > currentTimestamp.Seconds {
		return fmt.Errorf("auction with ID %s has not yet expired", auctionID)
	}

	sort.Slice(auction.Bids, func(i, j int) bool {
		return auction.Bids[i].BidPrice > auction.Bids[j].BidPrice
	})

	auction.IsActive = false

	resource, err := ac.fetchResource(ctx, auction.ResourceID)
	if err != nil {
		return err
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

	if err := ac.storeResource(ctx, auction.ResourceID, *resource); err != nil {
		return err
	}

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
