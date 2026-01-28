const hre = require("hardhat");
const fs = require("fs");
const path = require("path");

async function main() {
  const [deployer] = await hre.ethers.getSigners();
  const network = hre.network.name;
  const chainId = Number((await hre.ethers.provider.getNetwork()).chainId);

  console.log("Deploying contracts with account:", deployer.address);
  console.log("Network:", network);
  console.log("Chain ID:", chainId);

  // Deploy generic Bridge (no constructor parameters)
  console.log("\nDeploying generic Bridge contract...");
  console.log("  Supports any ERC20 token or native gas token");
  console.log("  Use address(0) for native tokens");

  const Bridge = await hre.ethers.getContractFactory("Bridge");
  const bridge = await Bridge.deploy();
  await bridge.waitForDeployment();

  const bridgeAddress = await bridge.getAddress();
  console.log("Bridge deployed to:", bridgeAddress);

  // Determine liquidity token based on network
  let liquidityToken;
  let liquidityInstructions;

  if (network === "sepolia") {
    // Sepolia: POL is ERC20 token
    liquidityToken = process.env.POL_TOKEN_SEPOLIA || "0x6a7c3f4b0651d6da389ad1d11d962ea458cdca70";
    console.log("\n⚠️  MANUAL STEP REQUIRED:");
    console.log("To add 50 POL (ERC20) liquidity on Sepolia:");
    console.log(`\n1. Approve Bridge to spend POL:`);
    console.log(`cast send ${liquidityToken} "approve(address,uint256)" ${bridgeAddress} 50000000000000000000 --private-key $PRIVATE_KEY --rpc-url $SEPOLIA_RPC_URL\n`);
    console.log(`2. Add liquidity:`);
    console.log(`cast send ${bridgeAddress} "addLiquidity(address,uint256)" ${liquidityToken} 50000000000000000000 --private-key $PRIVATE_KEY --rpc-url $SEPOLIA_RPC_URL\n`);
  } else if (network === "stavanger") {
    // Stavanger: POL is native token (address(0))
    liquidityToken = hre.ethers.ZeroAddress;
    console.log("\nAdding 50 native POL liquidity to bridge...");
    const liquidityAmount = hre.ethers.parseEther("50");

    // Call addLiquidity with native token (address(0))
    const tx = await bridge.addLiquidity(hre.ethers.ZeroAddress, liquidityAmount, {
      value: liquidityAmount
    });
    await tx.wait();
    console.log("✅ Added 50 POL liquidity to bridge");

    // Check balance
    const balance = await hre.ethers.provider.getBalance(bridgeAddress);
    console.log(`Bridge balance: ${hre.ethers.formatEther(balance)} POL`);
  } else {
    liquidityToken = hre.ethers.ZeroAddress;
    console.log("\n⚠️  Note: Add liquidity manually for this network");
  }

  // Get Pelagos contract address for this network
  const pelagosAddress = process.env[`PELAGOS_${network.toUpperCase()}`] || process.env.PELAGOS_CONTRACT || hre.ethers.ZeroAddress;
  if (pelagosAddress !== hre.ethers.ZeroAddress) {
    console.log("\nSetting Pelagos contract address:", pelagosAddress);
    const setPelagosTx = await bridge.setPelagosContract(pelagosAddress);
    await setPelagosTx.wait();
    console.log("✅ Pelagos contract set");
  } else {
    console.log("\n⚠️  Note: Pelagos contract address not set. Set it manually with:");
    console.log(`cast send ${bridgeAddress} "setPelagosContract(address)" <PELAGOS_ADDRESS> --private-key $PRIVATE_KEY --rpc-url $RPC_URL`);
  }

  // Save deployment info
  const deploymentInfo = {
    network: network,
    chainId: chainId,
    deployer: deployer.address,
    bridge: bridgeAddress,
    bridgeType: "generic",
    supportedTokens: "any ERC20 or native (address(0))",
    pelagosContract: pelagosAddress,
    deployedAt: new Date().toISOString(),
    blockNumber: await hre.ethers.provider.getBlockNumber()
  };

  // Create config directory if it doesn't exist
  const configDir = path.join(__dirname, "../../config");
  if (!fs.existsSync(configDir)) {
    fs.mkdirSync(configDir, { recursive: true });
  }

  // Load existing bridge contracts config or create new one
  const bridgeContractsPath = path.join(configDir, "bridge_contracts.json");
  let bridgeContracts = {
    version: "1.0.0",
    deployments: {}
  };

  if (fs.existsSync(bridgeContractsPath)) {
    bridgeContracts = JSON.parse(fs.readFileSync(bridgeContractsPath, "utf8"));
  }

  // Update with new deployment
  bridgeContracts.deployments[chainId.toString()] = {
    name: network,
    bridge: bridgeAddress,
    bridgeType: "generic",
    supportedTokens: "any ERC20 or native (address(0))",
    pelagosContract: pelagosAddress,
    deployedAt: deploymentInfo.blockNumber,
    deployer: deployer.address
  };

  // Save updated config
  fs.writeFileSync(
    bridgeContractsPath,
    JSON.stringify(bridgeContracts, null, 2)
  );

  console.log("\nDeployment info saved to:", bridgeContractsPath);

  // Save deployment details for this specific deployment
  const deploymentsDir = path.join(__dirname, "../deployments");
  if (!fs.existsSync(deploymentsDir)) {
    fs.mkdirSync(deploymentsDir, { recursive: true });
  }

  const deploymentFile = path.join(deploymentsDir, `${network}-${Date.now()}.json`);
  fs.writeFileSync(deploymentFile, JSON.stringify(deploymentInfo, null, 2));
  console.log("Detailed deployment saved to:", deploymentFile);

  // Print summary
  console.log("\n=== Deployment Summary ===");
  console.log("Network:", network);
  console.log("Chain ID:", chainId);
  console.log("Bridge:", bridgeAddress);
  console.log("Bridge Type: Generic (supports any token)");
  console.log("Native Token: address(0)");
  console.log("ERC20 Tokens: Any valid token contract");
  console.log("Pelagos Contract:", pelagosAddress === hre.ethers.ZeroAddress ? "Not set" : pelagosAddress);
  console.log("Deployed at block:", deploymentInfo.blockNumber);
  console.log("========================\n");

  // Verification instructions
  if (network !== "localhost" && network !== "hardhat") {
    console.log("To verify contract on block explorer:");
    console.log(`npx hardhat verify --network ${network} ${bridgeAddress}`);
  }

  // Next steps
  console.log("\n=== Next Steps ===");
  console.log("1. Update config/consensus_chains.json:");
  console.log(`   - Set StartBlock to: ${deploymentInfo.blockNumber}`);
  console.log(`   - Set Bridge address to: ${bridgeAddress}`);
  console.log("\n2. Update config/ext_networks.json:");
  console.log(`   - Set contractAddress to: ${bridgeAddress}`);
  console.log("\n3. Update application/state_transition.go:");
  if (network === "sepolia") {
    console.log(`   - BridgeContractAddressSepolia = "${bridgeAddress}"`);
  } else if (network === "stavanger") {
    console.log(`   - BridgeContractAddressStavanger = "${bridgeAddress}"`);
  }
  console.log("\n4. Set Pelagos contract address (if not set):");
  console.log(`   bridge.setPelagosContract(<PELAGOS_ADDRESS>)`);
  console.log("\n5. Bridge Usage:");
  console.log("   - For native tokens: bridgeAsset(address(0), amount, destChainId, recipient) + send msg.value");
  console.log("   - For ERC20 tokens: approve first, then bridgeAsset(tokenAddress, amount, destChainId, recipient)");
  console.log("==================\n");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
