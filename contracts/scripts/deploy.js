const hre = require("hardhat");
const fs = require("fs");
const path = require("path");

// Appchain ID for the bridge
const APP_CHAIN_ID = 1604;

async function main() {
  const [deployer] = await hre.ethers.getSigners();
  const network = hre.network.name;
  const chainId = Number((await hre.ethers.provider.getNetwork()).chainId);

  console.log("Deploying contracts with account:", deployer.address);
  console.log("Network:", network);
  console.log("Chain ID:", chainId);
  console.log("App Chain ID:", APP_CHAIN_ID);
  console.log("");

  // Step 1: Deploy Pelagos contract
  console.log("Step 1: Deploying Pelagos contract...");
  const Pelagos = await hre.ethers.getContractFactory("Pelagos");
  const pelagos = await Pelagos.deploy();
  await pelagos.waitForDeployment();
  const pelagosAddress = await pelagos.getAddress();
  console.log("  Pelagos deployed to:", pelagosAddress);

  // Step 2: Deploy Bridge contract
  console.log("\nStep 2: Deploying Bridge contract...");
  const Bridge = await hre.ethers.getContractFactory("Bridge");
  const bridge = await Bridge.deploy();
  await bridge.waitForDeployment();
  const bridgeAddress = await bridge.getAddress();
  console.log("  Bridge deployed to:", bridgeAddress);

  // Step 3: Set Pelagos contract on Bridge
  console.log("\nStep 3: Setting Pelagos contract on Bridge...");
  const setPelagosTx = await bridge.setPelagosContract(pelagosAddress);
  await setPelagosTx.wait();
  console.log("  Pelagos contract set on Bridge");

  // Step 4: Register Bridge with Pelagos as appchain contract
  console.log("\nStep 4: Registering Bridge with Pelagos (appchain ID:", APP_CHAIN_ID, ")...");
  const registerTx = await pelagos.registerAppchainContract(APP_CHAIN_ID, bridgeAddress);
  await registerTx.wait();
  console.log("  Bridge registered with Pelagos");

  // Verify registration
  const registeredAddress = await pelagos.appchainContracts(APP_CHAIN_ID);
  console.log("  Verified registered address:", registeredAddress);

  // Step 5: Add liquidity (Stavanger only - native POL)
  if (network === "stavanger") {
    console.log("\nStep 5: Adding 50 native POL liquidity to bridge...");
    const liquidityAmount = hre.ethers.parseEther("50");
    const liquidityTx = await bridge.addLiquidity(hre.ethers.ZeroAddress, liquidityAmount, {
      value: liquidityAmount
    });
    await liquidityTx.wait();
    console.log("  Added 50 POL liquidity");

    const balance = await hre.ethers.provider.getBalance(bridgeAddress);
    console.log("  Bridge balance:", hre.ethers.formatEther(balance), "POL");
  } else if (network === "sepolia") {
    const polToken = process.env.POL_TOKEN_SEPOLIA || "0x6a7c3f4b0651d6da389ad1d11d962ea458cdca70";
    console.log("\nStep 5: Manual liquidity required for Sepolia");
    console.log("  To add 50 POL (ERC20) liquidity:");
    console.log(`  1. Approve: cast send ${polToken} "approve(address,uint256)" ${bridgeAddress} 50000000000000000000 --private-key $PRIVATE_KEY --rpc-url $SEPOLIA_RPC_URL`);
    console.log(`  2. Add:     cast send ${bridgeAddress} "addLiquidity(address,uint256)" ${polToken} 50000000000000000000 --private-key $PRIVATE_KEY --rpc-url $SEPOLIA_RPC_URL`);
  }

  // Save deployment info
  const deploymentInfo = {
    network: network,
    chainId: chainId,
    appChainId: APP_CHAIN_ID,
    deployer: deployer.address,
    pelagos: pelagosAddress,
    bridge: bridgeAddress,
    deployedAt: new Date().toISOString(),
    blockNumber: await hre.ethers.provider.getBlockNumber()
  };

  // Create deployments directory if needed
  const deploymentsDir = path.join(__dirname, "../deployments");
  if (!fs.existsSync(deploymentsDir)) {
    fs.mkdirSync(deploymentsDir, { recursive: true });
  }

  // Save deployment file
  const deploymentFile = path.join(deploymentsDir, `${network}-${Date.now()}.json`);
  fs.writeFileSync(deploymentFile, JSON.stringify(deploymentInfo, null, 2));

  // Print summary
  console.log("\n=== Deployment Summary ===");
  console.log("Network:", network);
  console.log("Chain ID:", chainId);
  console.log("App Chain ID:", APP_CHAIN_ID);
  console.log("Pelagos:", pelagosAddress);
  console.log("Bridge:", bridgeAddress);
  console.log("Block:", deploymentInfo.blockNumber);
  console.log("Saved to:", deploymentFile);
  console.log("========================\n");

  // Verification instructions
  if (network !== "localhost" && network !== "hardhat") {
    console.log("To verify contracts on block explorer:");
    console.log(`  npx hardhat verify --network ${network} ${pelagosAddress}`);
    console.log(`  npx hardhat verify --network ${network} ${bridgeAddress}`);
  }

  // Next steps
  console.log("\n=== Next Steps ===");
  console.log("1. Update pelacli.yaml:");
  console.log(`   pelagos_contract: "${pelagosAddress}"`);
  console.log("\n2. Update config.yaml bridge_contracts:");
  console.log(`   ${chainId}: "${bridgeAddress}"`);
  console.log("\n3. Update frontend/app.js CONFIG:");
  console.log(`   BRIDGE_${network.toUpperCase()}: "${bridgeAddress}"`);
  console.log("==================\n");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
