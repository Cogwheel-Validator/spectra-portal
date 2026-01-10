"use client";

import { Wallet } from "lucide-react";
import { useState } from "react";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { useWallet } from "@/context/walletContext";
import WalletModal from "./walletModel";

interface WalletConnectProps {
    requiredChains?: ClientChain[]; // Chains that MUST be connected
    availableChains?: ClientChain[]; // Chains available for optional connection
}

export default function WalletConnect({
    requiredChains = [],
    availableChains = [],
}: WalletConnectProps): React.ReactNode {
    const { isConnected, connectedChainIds, getAddress, isConnectedToChain } = useWallet();
    const [isModalOpen, setIsModalOpen] = useState(false);

    // Check if all required chains are connected
    const allRequiredConnected =
        requiredChains.length === 0 || requiredChains.every((chain) => isConnectedToChain(chain.id));

    // Get missing required chains
    const missingRequiredChains = requiredChains.filter((chain) => !isConnectedToChain(chain.id));

    const displayText = () => {
        if (requiredChains.length === 0) {
            // No specific chains required - general wallet connection
            if (isConnected && connectedChainIds.length > 0) {
                const firstChainId = connectedChainIds[0];
                const address = getAddress(firstChainId);
                if (address) {
                    return `Connected (${address.slice(0, 6)}...${address.slice(-4)})`;
                }
                return `Connected (${connectedChainIds.length} chain${connectedChainIds.length !== 1 ? "s" : ""})`;
            }
            return "Connect Wallet";
        }

        // Specific chains required
        if (allRequiredConnected) {
            if (requiredChains.length === 1) {
                const address = getAddress(requiredChains[0].id);
                if (address) {
                    return `${requiredChains[0].name} (${address.slice(0, 6)}...${address.slice(-4)})`;
                }
                return `Connected to ${requiredChains[0].name}`;
            }
            return `Connected (${requiredChains.length} chains)`;
        }

        if (missingRequiredChains.length > 0) {
            if (missingRequiredChains.length === 1) {
                return `Connect to ${missingRequiredChains[0].name}`;
            }
            return `Connect ${missingRequiredChains.length} chains`;
        }

        return "Connect Wallet";
    };

    const getButtonStyle = () => {
        if (requiredChains.length === 0) {
            // No specific requirement - just show if connected
            return isConnected
                ? "flex flex-row btn btn-success btn-sm lg:btn-md"
                : "flex flex-row btn btn-primary btn-sm lg:btn-md";
        }

        // Has required chains
        if (allRequiredConnected) {
            return "flex flex-row btn btn-success btn-sm lg:btn-md";
        }

        if (isConnected && missingRequiredChains.length > 0) {
            // Connected but missing some required chains - show warning
            return "flex flex-row btn btn-warning btn-sm lg:btn-md";
        }

        // Not connected to required chains
        return "flex flex-row btn btn-primary btn-sm lg:btn-md";
    };

    return (
        <div>
            <button type="button" className={getButtonStyle()} onClick={() => setIsModalOpen(true)}>
                <Wallet size={24} strokeWidth={1.5} className="hidden lg:block" />
                <Wallet size={20} strokeWidth={1.5} className="lg:hidden" />
                <h6 className="font-bold hidden lg:block">{displayText()}</h6>
            </button>
            <WalletModal
                isOpen={isModalOpen}
                onClose={() => setIsModalOpen(false)}
                requiredChains={requiredChains}
                availableChains={availableChains}
            />
        </div>
    );
}
