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

func main() {
	chaincode, err := contractapi.NewChaincode(&EnergyAuctionContract{})
	if err != nil {
		log.Panicf("Error creating asset chaincode: %v", err)
	}

	if err := chaincode.Start(); err != nil {
		log.Panicf("Error starting asset chaincode: %v", err)
	}
}
