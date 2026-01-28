require("@nomicfoundation/hardhat-toolbox");
require("dotenv").config();

/** @type import('hardhat/config').HardhatUserConfig */
module.exports = {
  solidity: {
    version: "0.8.20",
    settings: {
      optimizer: {
        enabled: true,
        runs: 200
      }
    }
  },
  networks: {
    hardhat: {
      chainId: 31337
    },
    sepolia: {
      url: process.env.SEPOLIA_RPC_URL || "",
      accounts: process.env.PRIVATE_KEY ? [process.env.PRIVATE_KEY] : [],
      chainId: 11155111
    },
    stavanger: {
      url: process.env.STAVANGER_RPC_URL || "https://stavanger-rpc.eu-north-2.gateway.fm",
      accounts: process.env.PRIVATE_KEY ? [process.env.PRIVATE_KEY] : [],
      chainId: 50591822
    },
    amoy: {
      url: process.env.AMOY_RPC_URL || "https://rpc-amoy.polygon.technology",
      accounts: process.env.PRIVATE_KEY ? [process.env.PRIVATE_KEY] : [],
      chainId: 80002
    }
  },
  etherscan: {
    apiKey: {
      sepolia: process.env.ETHERSCAN_API_KEY || "",
      stavanger: process.env.STAVANGER_EXPLORER_API_KEY || "none", // May not have explorer
      polygonAmoy: process.env.POLYGONSCAN_API_KEY || ""
    },
    customChains: [
      {
        network: "stavanger",
        chainId: 50591822,
        urls: {
          apiURL: process.env.STAVANGER_EXPLORER_API_URL || "https://stavanger-blockscout.eu-north-2.gateway.fm/api",
          browserURL: process.env.STAVANGER_EXPLORER_URL || "https://stavanger-blockscout.eu-north-2.gateway.fm"
        }
      }
    ]
  }
};
