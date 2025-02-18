# Distributed Energy System Auctions

This project implements an auction mechanism for a distributed energy system using Hyperledger Fabric. Implemented as smart contracts, the project contains both an English Auction and a Second Price Auction. Furthermore, both have an associated optimized version that implements composite keys, batched writes, and pagination.

## Usage

1. Clone this repository:

```bash
git clone https://github.com/khalidzahra/fabric-auction-contracts.git
```

2. Clone the Hyperledger Fabric samples repository by running the following:

```bash
git clone https://github.com/hyperledger/fabric-samples.git
```

3. Launch the test network:

```bash
cd fabric-samples/test-network && ./network.sh up createChannel
```

4. Deploy the English Auction smart contract by running:

```bash
cp ../packageAndInstall.sh . && chmod +x packageAndInstall.sh && ./packageAndInstall.sh
```

Any contract can be deployed by using:

```bash
packageAndInstall.sh <chaincodeName> <chaincodePath>
```

5. Run a sample interaction script using:

```bash
cp ../invokeAuction.sh && chmod +x invokeAuction.sh && ./invokeAuction.sh
```

This can be run for any contract using:

```bash
invokeAuction.sh <chaincodeName> 
```
