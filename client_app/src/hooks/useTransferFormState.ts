import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState, useTransition } from "react";
import type { TransferMode } from "@/context/transferContext";

interface UseTransferFormStateReturn {
    sendChain: string;
    receiveChain: string;
    sendToken: string;
    receiveToken: string;
    amount: string;
    mode: TransferMode;
    isPending: boolean;
    setSendChain: (chain: string) => void;
    setReceiveChain: (chain: string) => void;
    setSendToken: (token: string) => void;
    setReceiveToken: (token: string) => void;
    setAmount: (amount: string) => void;
    setMode: (mode: TransferMode) => void;
    handleSendChainChange: (chainId: string) => void;
    handleReceiveChainChange: (chainId: string) => void;
    handleSendTokenChange: (tokenSymbol: string) => void;
    handleReceiveTokenChange: (tokenSymbol: string) => void;
    handleAmountChange: (value: string, debouncedUpdate: (value: string) => void) => void;
}

interface UseTransferFormStateProps {
    initialSendChain?: string;
    initialReceiveChain?: string;
    initialSendToken?: string;
    initialReceiveToken?: string;
    initialAmount?: string;
}

export function useTransferFormState({
    initialSendChain = "",
    initialReceiveChain = "",
    initialSendToken = "",
    initialReceiveToken = "",
    initialAmount = "",
}: UseTransferFormStateProps): UseTransferFormStateReturn {
    const router = useRouter();
    const searchParams = useSearchParams();
    const [isPending, startTransition] = useTransition();

    const [sendChain, setSendChain] = useState(initialSendChain);
    const [receiveChain, setReceiveChain] = useState(initialReceiveChain);
    const [sendToken, setSendToken] = useState(initialSendToken);
    const [receiveToken, setReceiveToken] = useState(initialReceiveToken);
    const [amount, setAmount] = useState(initialAmount);
    const [mode, setMode] = useState<TransferMode>("manual");

    const updateURL = useCallback(
        (
            updates: Partial<{
                from_chain: string;
                to_chain: string;
                send_asset: string;
                receive_asset: string;
                amount: string;
            }>,
        ) => {
            startTransition(() => {
                const params = new URLSearchParams(searchParams.toString());
                Object.entries(updates).forEach(([key, value]) => {
                    if (value !== undefined) {
                        if (value) params.set(key, value);
                        else params.delete(key);
                    }
                });
                router.push(`/transfer?${params.toString()}`, { scroll: false });
            });
        },
        [router, searchParams],
    );

    const handleSendChainChange = useCallback(
        (chainId: string) => {
            setSendChain(chainId);
            setSendToken("");
            setReceiveChain("");
            setReceiveToken("");
            setAmount("");
            updateURL({
                from_chain: chainId,
                send_asset: "",
                to_chain: "",
                receive_asset: "",
                amount: "",
            });
        },
        [updateURL],
    );

    const handleReceiveChainChange = useCallback(
        (chainId: string) => {
            setReceiveChain(chainId);
            setReceiveToken("");
            updateURL({ to_chain: chainId, receive_asset: "" });
        },
        [updateURL],
    );

    const handleSendTokenChange = useCallback(
        (tokenSymbol: string) => {
            setSendToken(tokenSymbol);
            updateURL({ send_asset: tokenSymbol });
        },
        [updateURL],
    );

    const handleReceiveTokenChange = useCallback(
        (tokenSymbol: string) => {
            setReceiveToken(tokenSymbol);
            updateURL({ receive_asset: tokenSymbol });
        },
        [updateURL],
    );

    const handleAmountChange = useCallback(
        (value: string, debouncedUpdate: (value: string) => void) => {
            setAmount(value);
            debouncedUpdate(value);
        },
        [],
    );

    return {
        sendChain,
        receiveChain,
        sendToken,
        receiveToken,
        amount,
        mode,
        isPending,
        setSendChain,
        setReceiveChain,
        setSendToken,
        setReceiveToken,
        setAmount,
        setMode,
        handleSendChainChange,
        handleReceiveChainChange,
        handleSendTokenChange,
        handleReceiveTokenChange,
        handleAmountChange,
    };
}

