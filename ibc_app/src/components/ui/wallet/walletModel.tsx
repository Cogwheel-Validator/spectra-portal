"use client";

import { Check, TriangleAlert } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useEffect, useState } from "react";
import type { ClientChain } from "@/components/modules/tomlTypes";
import { useWallet } from "@/context/walletContext";
import { WALLET_INFO, WalletType } from "@/lib/wallets/walletProvider";
import { getInstalledWallets } from "@/lib/wallets/walletUtility";

interface WalletModalProps {
    isOpen: boolean;
    onClose: () => void;
    requiredChains?: ClientChain[]; // Chains that MUST be connected
    availableChains?: ClientChain[]; // Chains available for optional connection
}

export default function WalletModal({
    isOpen,
    onClose,
    requiredChains = [],
    availableChains = [],
}: WalletModalProps) {
    const {
        isConnected,
        walletType: connectedWalletType,
        connection: { connect, disconnect },
        isConnectedToChain,
        getAddress,
        connectedChainIds,
    } = useWallet();

    const [isConnecting, setIsConnecting] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [installedWallets, setInstalledWallets] = useState<WalletType[]>([]);
    const [selectedWallet, setSelectedWallet] = useState<WalletType | null>(null);
    
    // Multi-chain selection state
    const [selectedChains, setSelectedChains] = useState<Set<string>>(new Set());
    const [showChainSelection, setShowChainSelection] = useState(false);

    // Detect installed wallets when modal opens
    useEffect(() => {
        if (isOpen) {
            const wallets = getInstalledWallets();
            setInstalledWallets(wallets);

            // If user is already connected, set the selected wallet to the connected one
            if (connectedWalletType) {
                setSelectedWallet(connectedWalletType);
            } else if (wallets.length > 0) {
                // Default to first installed wallet
                setSelectedWallet(wallets[0]);
            }

            // Initialize selected chains with required chains
            const initialSelection = new Set<string>();
            for (const chain of requiredChains) {
                initialSelection.add(chain.id);
            }
            setSelectedChains(initialSelection);
        }
    }, [isOpen, connectedWalletType, requiredChains]);

    const handleConnect = async (walletType: WalletType) => {
        if (isConnecting || selectedChains.size === 0) return;

        setIsConnecting(true);
        setError(null);
        try {
            // Get chain configs for selected chains
            const chainsToConnect: ClientChain[] = [];
            const allChains = [...requiredChains, ...availableChains];
            
            for (const chainId of selectedChains) {
                const chain = allChains.find((c) => c.id === chainId);
                if (chain) {
                    chainsToConnect.push(chain);
                }
            }

            if (chainsToConnect.length === 0) {
                setError("No chains selected");
                return;
            }

            await connect(chainsToConnect, walletType);
            onClose();
        } catch (error) {
            if (error instanceof Error) {
                const walletName = WALLET_INFO[walletType].name;
                if (error.message.includes("not installed")) {
                    setError(`Please install ${walletName}`);
                } else {
                    setError(error.message || "Failed to connect wallet");
                }
            } else {
                setError("Failed to connect wallet");
            }
            console.error("Failed to connect:", error);
        } finally {
            setIsConnecting(false);
        }
    };

    const handleChainToggle = (chainId: string, isRequired: boolean) => {
        if (isRequired) return; // Can't unselect required chains

        setSelectedChains((prev) => {
            const next = new Set(prev);
            if (next.has(chainId)) {
                next.delete(chainId);
            } else {
                next.add(chainId);
            }
            return next;
        });
    };

    const handleDisconnect = () => {
        disconnect();
        onClose();
    };

    const handleDisconnectChain = (chainId: string) => {
        disconnect([chainId]);
    };

    // Check if all required chains are connected
    const allRequiredConnected = requiredChains.every((chain) =>
        isConnectedToChain(chain.id),
    );

    // Get missing required chains
    const missingRequiredChains = requiredChains.filter(
        (chain) => !isConnectedToChain(chain.id),
    );

    if (!isOpen) return null;

    const allChains = [...requiredChains, ...availableChains];

    return (
        <div className="fixed inset-0 z-50">
            <button
                type="button"
                className="fixed inset-0 bg-black/30 cursor-default"
                onClick={onClose}
                aria-label="Close modal"
            />

            <div className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2">
                <div className="bg-base-300 p-6 rounded-lg shadow-xl w-[500px] max-h-[80vh] overflow-y-auto">
                    <div className="flex justify-between items-center mb-4">
                        <h3 className="text-xl font-bold">
                            {showChainSelection ? "Select Chains" : "Connect Wallet"}
                        </h3>
                        <button
                            type="button"
                            onClick={onClose}
                            className="btn btn-ghost btn-sm"
                            aria-label="Close"
                        >
                            ✕
                        </button>
                    </div>

                    {/* Missing required chains warning */}
                    {!allRequiredConnected && missingRequiredChains.length > 0 && (
                        <div className="p-3 rounded-lg mb-4 bg-warning/10 text-warning border border-warning/20">
                            <p className="text-sm font-semibold mb-1 inline-flex items-center">
                                <TriangleAlert className="mr-4" /> Required chains not connected:
                            </p>
                            <ul className="text-xs space-y-1">
                                {missingRequiredChains.map((chain) => (
                                    <li className="list-disc list-inside" key={chain.id}>{chain.name}</li>
                                ))}
                            </ul>
                        </div>
                    )}

                    {showChainSelection ? (
                        <div className="space-y-4">
                            <p className="text-sm text-base-content/70">
                                Select chains to connect (required chains are pre-selected):
                            </p>

                            {/* Required Chains */}
                            {requiredChains.length > 0 && (
                                <div>
                                    <h4 className="text-sm font-semibold mb-2 text-error">
                                        Required Chains
                                    </h4>
                                    <div className="space-y-2">
                                        {requiredChains.map((chain) => {
                                            const isConnected = isConnectedToChain(chain.id);
                                            return (
                                                <div
                                                    key={chain.id}
                                                    className={`p-3 bg-base-200 rounded-lg flex items-center gap-3 ${
                                                        isConnected ? "ring-2 ring-success" : ""
                                                    }`}
                                                >
                                                    <input
                                                        type="checkbox"
                                                        checked={true}
                                                        disabled={true}
                                                        className="checkbox checkbox-sm"
                                                    />
                                                    <Image
                                                        src={`${chain.chain_logo}`}
                                                        alt={`${chain.name} Logo`}
                                                        width={32}
                                                        height={32}
                                                        className="object-contain rounded-full"
                                                        loading="eager"
                                                    />
                                                    <div className="flex-1">
                                                        <h6 className="font-semibold text-sm">
                                                            {chain.name}
                                                        </h6>
                                                        <p className="text-xs text-base-content/70">
                                                            {chain.id}
                                                        </p>
                                                    </div>
                                                    {isConnected && (
                                                        <span className="text-success text-xs inline-flex items-center"><Check className="mr-2 lg:mr-4" /> Connected</span>
                                                    )}
                                                </div>
                                            );
                                        })}
                                    </div>
                                </div>
                            )}

                            {/* Optional Chains */}
                            {availableChains.length > 0 && (
                                <div>
                                    <h4 className="text-sm font-semibold mb-2 text-base-content/70">
                                        Optional Chains
                                    </h4>
                                    <div className="space-y-2">
                                        {availableChains.map((chain) => {
                                            const isSelected = selectedChains.has(chain.id);
                                            const isConnected = isConnectedToChain(chain.id);
                                            return (
                                                <button
                                                    key={chain.id}
                                                    type="button"
                                                    onClick={() => handleChainToggle(chain.id, false)}
                                                    className={`w-full p-3 bg-base-200 rounded-lg flex items-center gap-3 transition-colors hover:bg-base-100 ${
                                                        isConnected ? "ring-2 ring-success" : ""
                                                    }`}
                                                >
                                                    <input
                                                        type="checkbox"
                                                        checked={isSelected}
                                                        onChange={() => {}}
                                                        className="checkbox checkbox-sm"
                                                    />
                                                    <Image
                                                        src={`${chain.chain_logo}`}
                                                        alt={`${chain.name} Logo`}
                                                        width={32}
                                                        height={32}
                                                        className="object-contain rounded-full"
                                                        loading="eager"
                                                    />
                                                    <div className="flex-1 text-left">
                                                        <h6 className="font-semibold text-sm">
                                                            {chain.name}
                                                        </h6>
                                                        <p className="text-xs text-base-content/70">
                                                            {chain.id}
                                                        </p>
                                                    </div>
                                                    {isConnected && (
                                                        <span className="text-success text-xs inline-flex items-center"><Check className="mr-2 lg:mr-4" /> Connected</span>
                                                    )}
                                                </button>
                                            );
                                        })}
                                    </div>
                                </div>
                            )}

                            <button
                                type="button"
                                onClick={() => setShowChainSelection(false)}
                                className="w-full btn btn-primary"
                                disabled={selectedChains.size === 0}
                            >
                                Continue with {selectedChains.size} chain{selectedChains.size !== 1 ? "s" : ""}
                            </button>
                        </div>
                    ) : (
                        <div className="space-y-4">
                            {/* Show selected chains summary */}
                            {selectedChains.size > 0 && !isConnected && (
                                <div className="p-4 bg-base-200 rounded-lg">
                                    <div className="flex justify-between items-center mb-2">
                                        <h6 className="font-semibold text-sm text-base-content">
                                            Selected Chains ({selectedChains.size})
                                        </h6>
                                        {allChains.length > selectedChains.size && (
                                            <button
                                                type="button"
                                                onClick={() => setShowChainSelection(true)}
                                                className="text-xs link link-primary"
                                            >
                                                Change selection
                                            </button>
                                        )}
                                    </div>
                                    <div className="space-y-1 text-base-content">
                                        {Array.from(selectedChains).map((chainId) => {
                                            const chain = allChains.find((c) => c.id === chainId);
                                            return chain ? (
                                                <div key={chainId} className="text-xs flex items-center gap-2">
                                                    <Image
                                                        src={`${chain.chain_logo}`}
                                                        alt={chain.name}
                                                        width={16}
                                                        height={16}
                                                        className="rounded-full"
                                                        loading="eager"
                                                    />
                                                    {chain.name}
                                                </div>
                                            ) : null;
                                        })}
                                    </div>
                                </div>
                            )}

                            {/* Wallet Selection */}
                            {installedWallets.length === 0 ? (
                                <div className="p-4 bg-warning/10 border border-warning/20 rounded-lg">
                                    <p className="text-warning text-sm mb-3">
                                        No wallets detected. Please install one of the following:
                                    </p>
                                    <div className="space-y-2">
                                        {Object.values(WalletType).map((type) => {
                                            const info = WALLET_INFO[type];
                                            return (
                                                <Link
                                                    key={type}
                                                    href={info.downloadUrl}
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                    className="flex items-center gap-3 p-3 bg-base-200 rounded-lg hover:bg-base-100 transition-colors"
                                                >
                                                    <Image
                                                        src={info.logo}
                                                        alt={info.name}
                                                        width={32}
                                                        height={32}
                                                        className="rounded-md"
                                                        loading="eager"
                                                    />
                                                    <div className="text-sm">
                                                        <span className="font-semibold text-base-content">{info.name}</span>
                                                        <span className="text-base-content/50 ml-2">
                                                            - Download
                                                        </span>
                                                    </div>
                                                </Link>
                                            );
                                        })}
                                    </div>
                                </div>
                            ) : (
                                <div className="space-y-3">
                                    {installedWallets.map((walletType) => {
                                        const info = WALLET_INFO[walletType];
                                        const isConnectedWithThisWallet = connectedWalletType === walletType;

                                        return (
                                            <button
                                                key={walletType}
                                                type="button"
                                                onClick={() => handleConnect(walletType)}
                                                disabled={
                                                    isConnecting ||
                                                    (isConnected && !isConnectedWithThisWallet)
                                                }
                                                className={`w-full p-4 bg-base-200 rounded-lg transition-colors flex items-center gap-4 cursor-pointer
                                                    ${
                                                        isConnecting ||
                                                        (isConnected && !isConnectedWithThisWallet)
                                                            ? "opacity-50 cursor-not-allowed"
                                                            : "hover:bg-base-100"
                                                    }
                                                    ${isConnectedWithThisWallet ? "ring-2 ring-success" : ""}`}
                                            >
                                                <Image
                                                    src={info.logo}
                                                    alt={`${info.name} Logo`}
                                                    width={50}
                                                    height={50}
                                                    className="object-contain rounded-md"
                                                    loading="eager"
                                                />
                                                <div className="flex-1 text-left">
                                                    <h6 className="font-bold text-base-content">{info.name}</h6>
                                                    <p className="text-sm text-base-content/70">
                                                        {isConnecting && selectedWallet === walletType
                                                            ? "Connecting..."
                                                            : isConnectedWithThisWallet
                                                              ? `Connected to ${connectedChainIds.length} chain${connectedChainIds.length !== 1 ? "s" : ""}`
                                                              : `Connect to ${selectedChains.size} chain${selectedChains.size !== 1 ? "s" : ""}`}
                                                    </p>
                                                </div>
                                                {isConnectedWithThisWallet && (
                                                    <div className="text-success inline-flex items-center"><Check className="mr-2 lg:mr-4" /></div>
                                                )}
                                            </button>
                                        );
                                    })}
                                </div>
                            )}

                            {error && (
                                <div className="text-error text-sm mt-2 p-2 bg-error/10 rounded">
                                    <p>{error}</p>
                                    {error.includes("Please install") && selectedWallet && (
                                        <Link
                                            href={WALLET_INFO[selectedWallet].downloadUrl}
                                            target="_blank"
                                            rel="noopener noreferrer"
                                            className="link link-primary mt-1 inline-block"
                                        >
                                            Download {WALLET_INFO[selectedWallet].name}
                                        </Link>
                                    )}
                                </div>
                            )}

                            {isConnected && (
                                <div className="space-y-2">
                                    {/* Show connected chains with individual disconnect */}
                                    {connectedChainIds.length > 0 && (
                                        <div className="p-3 bg-base-200 rounded-lg">
                                            <h6 className="font-semibold text- text-base-content mb-2">Connected Chains:</h6>
                                            <div className="space-y-2">
                                                {connectedChainIds.map((chainId) => {
                                                    const chain = allChains.find((c) => c.id === chainId);
                                                    const address = getAddress(chainId);
                                                    const isRequired = requiredChains.some((c) => c.id === chainId);
                                                    
                                                    return (
                                                        <div
                                                            key={chainId}
                                                            className="flex items-center justify-between text-xs"
                                                        >
                                                            <div className="flex items-center gap-2">
                                                                {chain && (
                                                                    <Image
                                                                        src={`${chain.chain_logo}`}
                                                                        alt={chain.name}
                                                                        width={20}
                                                                        height={20}
                                                                        className="rounded-full"
                                                                        loading="eager"
                                                                    />
                                                                )}
                                                                <div>
                                                                    <p className="font-semibold text-xs">
                                                                        {chain?.name || chainId}
                                                                        {isRequired && (
                                                                            <span className="text-error ml-1">*</span>
                                                                        )}
                                                                    </p>
                                                                    {address && (
                                                                        <p className="text-xs text-base-content/70">
                                                                            {address.slice(0, 10)}...{address.slice(-6)}
                                                                        </p>
                                                                    )}
                                                                </div>
                                                            </div>
                                                            {!isRequired && (
                                                                <button
                                                                    type="button"
                                                                    onClick={() => handleDisconnectChain(chainId)}
                                                                    className="btn btn-ghost btn-xs text-error"
                                                                >
                                                                    Disconnect
                                                                </button>
                                                            )}
                                                        </div>
                                                    );
                                                })}
                                            </div>
                                        </div>
                                    )}

                                    <button
                                        type="button"
                                        onClick={handleDisconnect}
                                        className="w-full btn btn-error"
                                    >
                                        Disconnect All
                                    </button>
                                </div>
                            )}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
