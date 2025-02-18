#!/bin/bash

CHAINCODE_NAME=${1:-"auction"}
ORDERER_ADDRESS="localhost:7050"
ORDERER_TLS_HOSTNAME="orderer.example.com"
ORDERER_CA_FILE="${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
CHANNEL_NAME="mychannel"
PEER_ADDRESSES=("localhost:7051" "localhost:9051")
TLS_ROOT_CERT_FILES=(
  "${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt"
  "${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt"
)

invoke_chaincode() {
  local function_name=$1
  local args=$2

  echo "Invoking chaincode: $CHAINCODE_NAME function: $function_name with args: $args"
  start_time=$(date +%s%3N)  

  peer chaincode invoke \
    -o $ORDERER_ADDRESS \
    --ordererTLSHostnameOverride $ORDERER_TLS_HOSTNAME \
    --tls \
    --cafile $ORDERER_CA_FILE \
    -C $CHANNEL_NAME \
    -n $CHAINCODE_NAME \
    --peerAddresses ${PEER_ADDRESSES[0]} \
    --tlsRootCertFiles ${TLS_ROOT_CERT_FILES[0]} \
    --peerAddresses ${PEER_ADDRESSES[1]} \
    --tlsRootCertFiles ${TLS_ROOT_CERT_FILES[1]} \
    -c "{\"function\":\"$function_name\",\"Args\":$args}"

  end_time=$(date +%s%3N) 
  duration=$((end_time - start_time))
  
  if [ $? -ne 0 ]; then
    echo "Error: Failed to invoke chaincode for function $function_name with args $args"
    exit 1
  fi

  echo "Execution time for $function_name: $duration ms"
}

resources=(
  '["res1", "200", "0.5", "Generation"]'
  '["res2", "600", "2", "Generation"]'
  '["res3", "700", "0.5", "Storage"]'
)

for resource in "${resources[@]}"; do
  invoke_chaincode "SubmitEnergyResource" "$resource"
done

echo "All resources submitted successfully."

sleep 3

invoke_chaincode "GetMeritOrder" '[]'

sleep 3

invoke_chaincode "GetResource" '["res1"]'

sleep 3

invoke_chaincode "StartAuction" '["res1", "10"]'

sleep 3

invoke_chaincode "GetAuction" '["res1"]'

sleep 3

invoke_chaincode "Bid" '["res1", "20"]'

sleep 3

invoke_chaincode "GetAuction" '["res1"]'

sleep 3

invoke_chaincode "Bid" '["res1", "50"]'

sleep 3

invoke_chaincode "GetAuction" '["res1"]'

sleep 3

invoke_chaincode "Bid" '["res1", "70"]'

sleep 3

invoke_chaincode "GetAuction" '["res1"]'

sleep 4

invoke_chaincode "Bid" '["res1", "80"]'
