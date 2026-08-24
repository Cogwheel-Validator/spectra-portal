"use client";

import { ArrowDown, ArrowRight } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { ClientChain, ClientConfig, ClientToken } from "@/components/modules/tomlTypes";
import FromSection from "@/components/ui/send/fromSection";
import RouteDisplay from "@/components/ui/send/routeDisplay";
import ToSection from "@/components/ui/send/toSection";
import TransferButton from "@/components/ui/send/transferButton";
import TransferModeToggle from "@/components/ui/send/transferModeToggle";
import WalletConnect from "@/components/ui/wallet/walletConnect";
import { useTransfer } from "@/context/transferContext";
import { useWallet } from "@/context/walletContext";
import { useBalanceValidation } from "@/hooks/useBalanceValidation";
import { useDebouncedCallback } from "@/hooks/useDebounce";
import { usePathfinderQuery } from "@/hooks/usePathfinderQuery";
import { useRouteInfo } from "@/hooks/useRouteInfo";
import { useTransferFormState } from "@/hooks/useTransferFormState";
import {
    type ChainAddress,
    ResponseCode,
} from "@/lib/generated/pathfinder/v2beta/pathfinder_v2beta_find_path_pb";
import { humanToBaseUnits } from "@/lib/utils";

interface SendUIProps {
    config: ClientConfig;
    sendChain?: string;
    receiveChain?: string;
    sendToken?: string;
    receiveToken?: string;
    amount?: string;
    pathfinderUrl: string;
}

export default function SendUI({
    config,
    sendChain: initialSendChain = "",
    receiveChain: initialReceiveChain = "",
    sendToken: initialSendToken = "",
    receiveToken: initialReceiveToken = "",
    amount: initialAmount = "",
    pathfinderUrl,
}: SendUIProps) {
    const { isConnectedToChain, getAddress } = useWallet();
    const transfer = useTransfer();

    // Form state management
    const {
        sendChain,
        receiveChain,
        sendToken,
        receiveToken,
        amount,
        mode,
        isPending,
        setMode,
        handleSendChainChange,
        handleReceiveChainChange,
        handleSendTokenChange,
        handleReceiveTokenChange,
        handleAmountChange,
    } = useTransferFormState({
        initialSendChain,
        initialReceiveChain,
        initialSendToken,
        initialReceiveToken,
        initialAmount,
    });

    // Helper functions
    const getChainById = useCallback(
        (chainId: string): ClientChain | undefined => {
            return config.chains.find((chain) => chain.id === chainId);
        },
        [config],
    );

    const getTokensForChain = useCallback(
        (chainId: string): ClientToken[] => {
            const chain = getChainById(chainId);
            if (!chain) return [];
            return [...chain.native_tokens, ...chain.ibc_tokens];
        },
        [getChainById],
    );

    const getConnectedChains = useCallback(
        (chainId: string): ClientChain[] => {
            const chain = getChainById(chainId);
            if (!chain) return [];
            return chain.connected_chains
                .map((cc) => getChainById(cc.id))
                .filter((c): c is ClientChain => c !== undefined);
        },
        [getChainById],
    );

    // Derived data
    const sendChainData = useMemo(() => getChainById(sendChain), [sendChain, getChainById]);
    const receiveChainData = useMemo(
        () => getChainById(receiveChain),
        [receiveChain, getChainById],
    );

    // Find broker chains (DEX chains like Osmosis that can swap and route)
    const brokerChains = useMemo(() => {
        return config.chains.filter((c) => c.is_dex);
    }, [config.chains]);

    // Get all chains reachable from source chain (directly or via broker)
    const availableReceiveChains = useMemo(() => {
        if (!sendChain) return [];

        const directlyConnected = getConnectedChains(sendChain);
        const directIds = new Set(directlyConnected.map((c) => c.id));

        // Add chains reachable via broker chains
        const viaBroker: ClientChain[] = [];
        for (const broker of brokerChains) {
            const sourceChain = getChainById(sendChain);
            const canReachBroker = sourceChain?.connected_chains.some((cc) => cc.id === broker.id);

            if (canReachBroker || sendChain === broker.id) {
                for (const connected of broker.connected_chains) {
                    if (!directIds.has(connected.id) && connected.id !== sendChain) {
                        const chain = getChainById(connected.id);
                        if (chain && !viaBroker.some((c) => c.id === chain.id)) {
                            viaBroker.push(chain);
                        }
                    }
                }
            }
        }

        return [...directlyConnected, ...viaBroker];
    }, [sendChain, getConnectedChains, brokerChains, getChainById]);

    // Base tokens available on chains (without balance sorting)
    const baseSendTokens = useMemo(() => {
        return getTokensForChain(sendChain);
    }, [sendChain, getTokensForChain]);

    const availableReceiveTokens = useMemo(() => {
        return getTokensForChain(receiveChain);
    }, [receiveChain, getTokensForChain]);

    // Wallet addresses
    const senderAddress = useMemo(() => getAddress(sendChain) ?? "", [sendChain, getAddress]);
    const receiverAddress = useMemo(
        () => getAddress(receiveChain) ?? "",
        [receiveChain, getAddress],
    );

    // Balance validation hook - get sender balance for sorting tokens
    const { senderBalance } = useBalanceValidation(sendChain, senderAddress, null, amount);

    // All tokens available on source chain, sorted by user's balance
    const availableSendTokens = useMemo(() => {
        if (senderBalance?.balances) {
            const balanceMap = new Map(
                senderBalance.balances.map((b: { denom: string; amount: string }) => [
                    b.denom,
                    BigInt(b.amount),
                ]),
            );

            return [...baseSendTokens].sort((a, b) => {
                const balanceA = balanceMap.get(a.denom) ?? BigInt(0);
                const balanceB = balanceMap.get(b.denom) ?? BigInt(0);
                if (balanceB > balanceA) return 1;
                if (balanceB < balanceA) return -1;
                return 0;
            });
        }

        return baseSendTokens;
    }, [baseSendTokens, senderBalance]);

    const selectedSendToken = useMemo(
        () => availableSendTokens.find((t) => t.denom === sendToken) ?? null,
        [availableSendTokens, sendToken],
    );

    const selectedReceiveToken = useMemo(
        () => availableReceiveTokens.find((t) => t.denom === receiveToken) ?? null,
        [availableReceiveTokens, receiveToken],
    );

    // Recompute balance validation with selected token
    const {
        formattedBalance: finalFormattedBalance,
        insufficientBalance: finalInsufficientBalance,
        balanceLoading: finalBalanceLoading,
    } = useBalanceValidation(sendChain, senderAddress, selectedSendToken, amount);

    // Convert amount to base units (with decimals) for pathfinder
    const amountInBaseUnits = useMemo(() => {
        return humanToBaseUnits(amount, selectedSendToken?.decimals ?? 6);
    }, [amount, selectedSendToken]);

    const slippageBps = transfer.state.slippageBps;

    // Chain IDs that need a wallet address, per the v2beta FindPath discovery
    // response. `transfer.state.pathfinderResponse` holds the previous
    // render's quote (synced below), which is what lets us know which
    // chains are required before issuing this render's request - reading
    // this render's own query result here would be a temporal-dead-zone
    // problem, since that result depends on the params we're building now.
    // Before any response has come back, fall back to just the two
    // endpoints the user picked.
    const requiredChainIds = useMemo(() => {
        const previousResponse = transfer.state.pathfinderResponse;
        if (previousResponse?.requiredChains && previousResponse.requiredChains.length > 0) {
            return previousResponse.requiredChains;
        }
        return [sendChain, receiveChain].filter(Boolean);
    }, [transfer.state.pathfinderResponse, sendChain, receiveChain]);

    // Once every required chain has a connected wallet address, build the
    // real ChainAddress list for an executable route. Until then this stays
    // empty, which keeps the pathfinder query in read-only discovery mode.
    const chainAddresses = useMemo((): ChainAddress[] => {
        if (requiredChainIds.length === 0) return [];
        const addresses: ChainAddress[] = [];
        for (const chainId of requiredChainIds) {
            const address = getAddress(chainId);
            if (!address) return [];
            addresses.push({ chainId, address } as ChainAddress);
        }
        return addresses;
    }, [requiredChainIds, getAddress]);

    const addressesPresent = chainAddresses.length > 0;

    // Base parameters for pathfinder queries
    const basePathfinderParams = useMemo(() => {
        if (!sendChain || !receiveChain || !sendToken) {
            return null;
        }
        return {
            chainFrom: sendChain,
            tokenFromDenom: selectedSendToken?.denom ?? "",
            amountIn: amountInBaseUnits,
            chainTo: receiveChain,
            tokenToDenom: selectedReceiveToken?.denom ?? "",
            addresses: chainAddresses,
            slippageBps: slippageBps,
        };
    }, [
        sendChain,
        receiveChain,
        sendToken,
        amountInBaseUnits,
        chainAddresses,
        selectedSendToken,
        selectedReceiveToken,
        slippageBps,
    ]);

    // Separate parameters for smart and manual modes
    const smartPathfinderParams = useMemo(() => {
        if (!basePathfinderParams) return null;
        return { ...basePathfinderParams, smartRoute: true };
    }, [basePathfinderParams]);

    const manualPathfinderParams = useMemo(() => {
        if (!basePathfinderParams) return null;
        return { ...basePathfinderParams, smartRoute: false };
    }, [basePathfinderParams]);

    // Ready to query as soon as the route shape is known - a route can be
    // discovered (in mock-address mode) before any wallet is connected.
    const isReadyToQuery = !!(
        sendChain &&
        receiveChain &&
        sendToken &&
        amount &&
        Number.parseFloat(amount) > 0
    );

    const queryOptions = {
        debounceMs: 2000,
        autoRefreshMs: 20000,
        staleAfterMs: 15000,
    };

    // Separate queries for smart and manual modes
    const smartQuery = usePathfinderQuery(
        smartPathfinderParams,
        isReadyToQuery && mode === "smart",
        queryOptions,
        pathfinderUrl,
    );

    const manualQuery = usePathfinderQuery(
        manualPathfinderParams,
        isReadyToQuery && mode === "manual",
        queryOptions,
        pathfinderUrl,
    );

    // Select the appropriate query based on current mode
    const {
        data: pathfinderResponse,
        isLoading: routeLoading,
        isPending: routePending,
        error: routeError,
        isStale: routeIsStale,
        quoteAgeSeconds,
        refetchFresh,
    } = mode === "smart" ? smartQuery : manualQuery;

    // Route information hook
    const { routeInfo, supportsPfm, supportsWasm, isDirectRoute } = useRouteInfo(
        pathfinderResponse,
        mode,
    );

    // Extract stable setters from transfer context
    const {
        setPathfinderResponse,
        setFromChain,
        setToChain,
        setFromToken,
        setToToken,
        setAmount: setTransferAmount,
        setSenderAddress,
        setReceiverAddress,
        setMode: setTransferMode,
        setSlippage,
    } = transfer;

    // Update transfer context when pathfinder response changes
    useEffect(() => {
        setPathfinderResponse(pathfinderResponse);
    }, [pathfinderResponse, setPathfinderResponse]);

    // Sync transfer context with local state
    useEffect(() => {
        setFromChain(sendChain);
        setToChain(receiveChain);
        setFromToken(selectedSendToken);
        setToToken(selectedReceiveToken);
        setTransferAmount(amount);
        setSenderAddress(senderAddress);
        setReceiverAddress(receiverAddress);
        setTransferMode(mode);
    }, [
        sendChain,
        receiveChain,
        selectedSendToken,
        selectedReceiveToken,
        amount,
        senderAddress,
        receiverAddress,
        mode,
        setFromChain,
        setToChain,
        setFromToken,
        setToToken,
        setTransferAmount,
        setSenderAddress,
        setReceiverAddress,
        setTransferMode,
    ]);

    // Required chains for wallet connection. Sourced from the pathfinder's
    // own `required_chains` (populated once a discovery-mode quote comes
    // back), which already includes source, destination, and any
    // intermediate broker/PFM chains - falling back to just the two
    // endpoints before a quote has been returned.
    const requiredChains = useMemo((): ClientChain[] => {
        const chains = requiredChainIds
            .map((chainId) => getChainById(chainId))
            .filter((chain): chain is ClientChain => chain !== undefined);
        if (chains.length > 0) return chains;

        const fallback: ClientChain[] = [];
        if (sendChainData) fallback.push(sendChainData);
        if (receiveChainData && receiveChainData.id !== sendChainData?.id) {
            fallback.push(receiveChainData);
        }
        return fallback;
    }, [requiredChainIds, getChainById, sendChainData, receiveChainData]);

    // Validation
    const isWalletReady = useMemo(() => {
        const nRequired = requiredChains.length;
        const missingChains = requiredChains.filter((chain) => !isConnectedToChain(chain.id));
        if (missingChains.length > 0)
            return { ready: false, missingChains, multiHop: nRequired > 2 };
        return { ready: true, missingChains: [], multiHop: nRequired > 2 };
    }, [requiredChains, isConnectedToChain]);

    const canSubmit = useMemo(() => {
        return (
            isWalletReady.ready &&
            pathfinderResponse?.success === true &&
            // A mock-address discovery response can't be executed - only a
            // real, address-backed quote (RESPONSE_CODE_OK) can be submitted.
            pathfinderResponse?.responseCode === ResponseCode.OK &&
            !routeLoading &&
            !routePending &&
            Number.parseFloat(amount) > 0 &&
            !finalInsufficientBalance &&
            !finalBalanceLoading
        );
    }, [
        isWalletReady,
        pathfinderResponse,
        routeLoading,
        routePending,
        amount,
        finalInsufficientBalance,
        finalBalanceLoading,
    ]);

    const { startPreparing } = transfer;
    const [isRefreshing, setIsRefreshing] = useState(false);

    const handleSubmit = useCallback(async () => {
        if (!canSubmit || !pathfinderResponse) return;

        // If quote is stale, refresh it first
        if (routeIsStale) {
            setIsRefreshing(true);
            try {
                const freshResponse = await refetchFresh();
                if (!freshResponse?.success || freshResponse.responseCode !== ResponseCode.OK) {
                    setIsRefreshing(false);
                    return;
                }
                setPathfinderResponse(freshResponse);
            } catch {
                setIsRefreshing(false);
                return;
            }
            setIsRefreshing(false);
        }

        startPreparing();
    }, [
        canSubmit,
        pathfinderResponse,
        routeIsStale,
        refetchFresh,
        setPathfinderResponse,
        startPreparing,
    ]);

    // Debounced URL update for amount
    const debouncedUpdateURL = useDebouncedCallback((_value: string) => {
        // This is handled inside useTransferFormState now
    }, 2000);

    const wrappedHandleAmountChange = useCallback(
        (value: string) => {
            handleAmountChange(value, debouncedUpdateURL);
        },
        [handleAmountChange, debouncedUpdateURL],
    );

    return (
        <div className="space-y-4 lg:space-y-5">
            {/* Header */}
            <div className="flex justify-between items-center">
                <h1 className="text-2xl lg:text-3xl font-bold text-white">Transfer Assets</h1>
                <WalletConnect requiredChains={requiredChains} availableChains={config.chains} />
            </div>

            {/* Main Transfer Section */}
            <div className="flex flex-col lg:flex-row lg:items-stretch gap-4 lg:gap-6">
                <FromSection
                    chains={config.chains}
                    availableSendTokens={availableSendTokens}
                    selectedSendToken={selectedSendToken}
                    sendChain={sendChain}
                    sendToken={sendToken}
                    amount={amount}
                    senderAddress={senderAddress}
                    receiveChain={receiveChain}
                    receiveToken={receiveToken}
                    formattedBalance={finalFormattedBalance}
                    insufficientBalance={finalInsufficientBalance}
                    balanceLoading={finalBalanceLoading}
                    isPending={isPending}
                    onSendChainChange={handleSendChainChange}
                    onSendTokenChange={handleSendTokenChange}
                    onAmountChange={wrappedHandleAmountChange}
                    senderBalance={senderBalance}
                />

                {/* Arrow Separator */}
                <div className="flex justify-center items-center lg:self-center">
                    <div className="p-2 lg:p-3 bg-slate-700/50 rounded-full">
                        <ArrowDown className="w-5 h-5 lg:hidden text-slate-400" />
                        <ArrowRight className="w-5 h-5 hidden lg:block text-slate-400" />
                    </div>
                </div>

                <ToSection
                    availableReceiveChains={availableReceiveChains}
                    availableReceiveTokens={availableReceiveTokens}
                    receiveChain={receiveChain}
                    receiveToken={receiveToken}
                    sendChain={sendChain}
                    isPending={isPending}
                    onReceiveChainChange={handleReceiveChainChange}
                    onReceiveTokenChange={handleReceiveTokenChange}
                />
            </div>

            {/* Route Info + Mode + Submit */}
            <div className="space-y-3 lg:space-y-4">
                {/* Transfer Mode Toggle */}
                {pathfinderResponse?.success && (
                    <div className="bg-slate-800/30 rounded-xl p-4 lg:p-5 border border-slate-700/50">
                        <TransferModeToggle
                            mode={mode}
                            onModeChange={setMode}
                            slippageBps={slippageBps}
                            onSlippageChange={setSlippage}
                            supportsPfm={supportsPfm}
                            supportsWasm={supportsWasm}
                            isDirectRoute={isDirectRoute}
                            disabled={isPending}
                        />
                    </div>
                )}

                {/* Route Display */}
                <RouteDisplay
                    addressesPresent={addressesPresent}
                    routeLoading={routeLoading}
                    routePending={routePending}
                    routeError={routeError}
                    routeInfo={routeInfo}
                    routeIsStale={routeIsStale}
                    quoteAgeSeconds={quoteAgeSeconds}
                    selectedReceiveToken={selectedReceiveToken}
                    amount={amount}
                />

                {/* Submit Button */}
                <TransferButton
                    canSubmit={canSubmit}
                    isPending={isPending}
                    isRefreshing={isRefreshing}
                    isWalletReady={isWalletReady}
                    pathfinderSuccess={pathfinderResponse?.success ?? false}
                    routeLoading={routeLoading}
                    routePending={routePending}
                    routeIsStale={routeIsStale}
                    mode={mode}
                    onSubmit={handleSubmit}
                />
            </div>
        </div>
    );
}
