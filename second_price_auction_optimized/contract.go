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

const (
	resourceObjectType = "resource"
	auctionObjectType  = "auction"
)

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
	fetchedResource, err := ac.fetchResource(ctx, resourceID)
	if err != nil {
		return "", err
	}
	return ac.marshalToString(fetchedResource)
}

func (ac *EnergyAuctionContract) GetMeritOrder(ctx contractapi.TransactionContextInterface) ([]EnergyResource, error) {
	results, err := ctx.GetStub().GetStateByPartialCompositeKey(resourceObjectType, []string{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve resources: %v", err)
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

func (ac *EnergyAuctionContract) GetMeritOrderPaginated(ctx contractapi.TransactionContextInterface, pageSize int32, bookmark string) ([]EnergyResource, string, error) {
	results, metadata, err := ctx.GetStub().GetStateByPartialCompositeKeyWithPagination(resourceObjectType, []string{}, pageSize, bookmark)
	if err != nil {
		return nil, "", fmt.Errorf("failed to retrieve resources: %v", err)
	}
	defer results.Close()

	var resources []EnergyResource
	for results.HasNext() {
		next, err := results.Next()
		if err != nil {
			return nil, "", err
		}

		_, splitKey, err := ctx.GetStub().SplitCompositeKey(next.Key)
		if err != nil {
			return nil, "", err
		}
		resourceID := splitKey[len(splitKey)-1]

		resource, err := ac.fetchResource(ctx, resourceID)
		if err != nil {
			return nil, "", err
		}

		resources = append(resources, *resource)
	}

	return resources, metadata.Bookmark, nil
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

	updates := make(map[string][]byte)

	resourceKey := ac.createCompositeKey(ctx, resourceObjectType, resourceID)
	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal resource: %v", err)
	}
	updates[resourceKey] = resourceJSON

	auctionKey := ac.createCompositeKey(ctx, auctionObjectType, resourceID)
	auctionJSON, err := json.Marshal(auction)
	if err != nil {
		return fmt.Errorf("failed to marshal auction: %v", err)
	}
	updates[auctionKey] = auctionJSON

	return ac.batchStore(ctx, updates)
}

func (ac *EnergyAuctionContract) GetAuction(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	auction, err := ac.fetchAuction(ctx, resourceID)
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
	auction, err := ac.fetchAuction(ctx, resourceID)
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
		return fmt.Errorf("auction for resource with ID %s is not active", resourceID)
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
		BidID:      fmt.Sprintf("%s:%s:%d", resourceID, clientID, currentTimestamp.Seconds),
		ResourceID: resourceID,
		Bidder:     clientID,
		BidPrice:   bidAmount,
		Timestamp:  currentTimestamp.Seconds,
	}

	auction.Bids = append(auction.Bids, bid)

	return ac.storeAuction(ctx, resourceID, *auction)
}

func (ac *EnergyAuctionContract) EndAuction(ctx contractapi.TransactionContextInterface, resourceID string) error {
	auction, err := ac.fetchAuction(ctx, resourceID)
	if err != nil {
		return err
	}

	if !auction.IsActive {
		return fmt.Errorf("auction for resource with ID %s is not active", resourceID)
	}

	currentTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("failed to get current block timestamp: %v", err)
	}

	if auction.Deadline > currentTimestamp.Seconds {
		return fmt.Errorf("auction for resource with ID %s has not yet expired", resourceID)
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

	updates := make(map[string][]byte)

	auctionKey := ac.createCompositeKey(ctx, auctionObjectType, resourceID)
	auctionJSON, err := json.Marshal(auction)
	if err != nil {
		return fmt.Errorf("failed to marshal auction: %v", err)
	}
	updates[auctionKey] = auctionJSON

	resourceKey := ac.createCompositeKey(ctx, resourceObjectType, resourceID)
	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal resource: %v", err)
	}
	updates[resourceKey] = resourceJSON

	return ac.batchStore(ctx, updates)
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
	resourceKey := ac.createCompositeKey(ctx, resourceObjectType, resourceID)

	fetchedResource, err := ctx.GetStub().GetState(resourceKey)
	if err != nil {
		return fmt.Errorf("failed to interact with world state: %v", err)
	}
	if fetchedResource != nil {
		return fmt.Errorf("a resource already exists with ID: %s", resourceID)
	}
	return nil
}

func (ac *EnergyAuctionContract) fetchResource(ctx contractapi.TransactionContextInterface, resourceID string) (*EnergyResource, error) {
	resourceKey := ac.createCompositeKey(ctx, resourceObjectType, resourceID)

	fetchedResource, err := ctx.GetStub().GetState(resourceKey)
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

func (ac *EnergyAuctionContract) fetchAuction(ctx contractapi.TransactionContextInterface, resourceID string) (*EnergyAuction, error) {
	auctionKey := ac.createCompositeKey(ctx, auctionObjectType, resourceID)

	fetchedAuction, err := ctx.GetStub().GetState(auctionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve auction: %v", err)
	}
	if fetchedAuction == nil {
		return nil, fmt.Errorf("auction for resource with ID %s does not exist", resourceID)
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

func (ac *EnergyAuctionContract) storeAuction(ctx contractapi.TransactionContextInterface, resourceID string, auction EnergyAuction) error {
	auctionKey := ac.createCompositeKey(ctx, auctionObjectType, resourceID)
	return ac.storeObject(ctx, auctionKey, auction)
}

func (ac *EnergyAuctionContract) storeResource(ctx contractapi.TransactionContextInterface, resourceID string, resource EnergyResource) error {
	resourceKey := ac.createCompositeKey(ctx, resourceObjectType, resourceID)
	return ac.storeObject(ctx, resourceKey, resource)
}

func (ac *EnergyAuctionContract) batchStore(ctx contractapi.TransactionContextInterface, updates map[string][]byte) error {
	for key, value := range updates {
		if err := ctx.GetStub().PutState(key, value); err != nil {
			return fmt.Errorf("failed to update state for key %s: %v", key, err)
		}
	}
	return nil
}

func (ac *EnergyAuctionContract) createCompositeKey(ctx contractapi.TransactionContextInterface, objectType string, objectAttributes ...string) string {
	key, _ := ctx.GetStub().CreateCompositeKey(objectType, objectAttributes)
	return key
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
