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
  echo $duration >> "${function_name}_latencies.txt"
}

calculate_average_latency() {
  local function_name=$1
  local latencies_file="${function_name}_latencies.txt"
  
  if [ ! -f "$latencies_file" ]; then
    echo "No latency data found for $function_name"
    return
  fi

  total_latency=0
  count=0

  while read -r latency; do
    total_latency=$((total_latency + latency))
    count=$((count + 1))
  done < "$latencies_file"

  if [ $count -eq 0 ]; then
    echo "No latency data found for $function_name"
    return
  fi

  average_latency=$((total_latency / count))
  echo "Average latency for $function_name: $average_latency ms"
}

for i in {1..1000}; do
  resource_id="res$i"
  resource_capacity=$((RANDOM % 1000 + 1))
  resource_cost=$((RANDOM % 100 + 1))
  resource_type=$((RANDOM % 2 == 0 ? "Generation" : "Storage"))
  
  invoke_chaincode "SubmitEnergyResource" "[\"$resource_id\", \"$resource_capacity\", \"$resource_cost\", \"$resource_type\"]" &
done

wait
echo "All resources submitted successfully."

for i in {1..1000}; do
  resource_id="res$i"
  invoke_chaincode "StartAuction" "[\"$resource_id\", \"10000\"]"
  sleep 1
done

for i in {1..100}; do
  resource_id="res$i"
  invoke_chaincode "EndAuction" "[\"$resource_id\"]"
  sleep 1
done

for i in {1..100}; do
  invoke_chaincode "GetMeritOrder" '[]'
  sleep 1
done

for i in {1..100}; do
  invoke_chaincode "GetMeritOrderPaginated" '["10", ""]'
  sleep 1
done

# Calculate average latencies
calculate_average_latency "StartAuction"
calculate_average_latency "EndAuction"
calculate_average_latency "GetMeritOrder"
calculate_average_latency "GetMeritOrderPaginated"