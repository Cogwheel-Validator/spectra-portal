import { useMemo } from "react";
import type { ClientToken } from "@/components/modules/tomlTypes";
import { useGetAddressBalance } from "@/lib/apiQueries/fetchApiData";

interface UseBalanceValidationReturn {
    tokenBalance: string | null;
    formattedBalance: string | null;
    insufficientBalance: boolean;
    balanceLoading: boolean;
    senderBalance: any;
}

export function useBalanceValidation(
    sendChain: string,
    senderAddress: string,
    selectedSendToken: ClientToken | null,
    amount: string,
): UseBalanceValidationReturn {
    const { data: senderBalance, isLoading: balanceLoading } = useGetAddressBalance(
        sendChain,
        senderAddress,
    );

    const tokenBalance = useMemo(() => {
        if (!senderBalance || !selectedSendToken) return null;
        const balance = senderBalance.balances.find((b) => b.denom === selectedSendToken.denom);
        return balance ? balance.amount : "0";
    }, [senderBalance, selectedSendToken]);

    const formattedBalance = useMemo(() => {
        if (!tokenBalance || !selectedSendToken) return null;
        const decimals = selectedSendToken.decimals ?? 6;
        const value = Number(tokenBalance) / 10 ** decimals;
        return value.toLocaleString(undefined, {
            minimumFractionDigits: 0,
            maximumFractionDigits: decimals > 6 ? 6 : decimals,
        });
    }, [tokenBalance, selectedSendToken]);

    const insufficientBalance = useMemo(() => {
        if (!tokenBalance || !amount || !selectedSendToken) return false;
        const decimals = selectedSendToken.decimals ?? 6;
        const amountInSmallestUnit = BigInt(Math.floor(Number(amount) * 10 ** decimals));
        const balanceInSmallestUnit = BigInt(tokenBalance);
        return amountInSmallestUnit > balanceInSmallestUnit;
    }, [tokenBalance, amount, selectedSendToken]);

    return {
        tokenBalance,
        formattedBalance,
        insufficientBalance,
        balanceLoading,
        senderBalance,
    };
}

