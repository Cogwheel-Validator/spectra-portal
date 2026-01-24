/**
 * Tests for IndexedDB Manager
 * Uses fake-indexeddb to simulate browser IndexedDB
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import "fake-indexeddb/auto";
import {
    AddTransactionToDb,
    ClearAllTransactions,
    ConnectToDb,
    DeleteDatabase,
    DeleteTransactionById,
    FetchTransactionById,
    LoadAllTransactions,
    type TransactionRecord,
    UpdateTransactionInDb,
    UpdateTransactionStatus,
} from "../src/lib/indexDb/dbManager";

describe("IndexedDB Manager", () => {
    let db: IDBDatabase;

    beforeEach(async () => {
        // Connect to database before each test
        db = await ConnectToDb();
    });

    afterEach(async () => {
        // Clean up after each test
        if (db) {
            db.close();
        }
        await DeleteDatabase();
    });

    describe("ConnectToDb", () => {
        it("should successfully open a database connection", async () => {
            expect(db).toBeDefined();
            expect(db.name).toBe("spectra_ibc");
            expect(db.version).toBe(1);
        });

        it("should create transactions object store", async () => {
            expect(db.objectStoreNames.contains("transactions")).toBe(true);
        });

        it("should create indexes on transactions store", async () => {
            const tx = db.transaction("transactions", "readonly");
            const store = tx.objectStore("transactions");
            expect(store.indexNames.contains("timestamp")).toBe(true);
            expect(store.indexNames.contains("status")).toBe(true);
        });
    });

    describe("AddTransactionToDb", () => {
        it("should add a transaction and return auto-incremented id", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "in-progress",
                totalSteps: 3,
                currentStep: 1,
                trajectory: ["cosmoshub-4", "osmosis-1"],
                error: null,
            };

            const id = await AddTransactionToDb(db, transaction);
            expect(id).toBe(1);
        });

        it("should auto-increment ids for multiple transactions", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "in-progress",
                totalSteps: 3,
                currentStep: 1,
                trajectory: null,
                error: null,
            };

            const id1 = await AddTransactionToDb(db, transaction);
            const id2 = await AddTransactionToDb(db, transaction);
            const id3 = await AddTransactionToDb(db, transaction);

            expect(id1).toBe(1);
            expect(id2).toBe(2);
            expect(id3).toBe(3);
        });

        it("should store all transaction fields correctly", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: "2026-01-09T12:00:00Z",
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1abc",
                toChainId: "atomone-1",
                toChainAddress: "atone1...",
                tokenIn: "ATOM",
                tokenOut: "ATONE",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "manual",
                status: "success",
                totalSteps: 1,
                currentStep: 1,
                trajectory: ["cosmoshub-4", "osmosis-1", "atomone-1"],
                error: null,
            };

            const id = await AddTransactionToDb(db, transaction);
            const retrieved = await FetchTransactionById(db, id);

            expect(retrieved).toMatchObject(transaction);
            expect(retrieved.id).toBe(id);
        });
    });

    describe("FetchTransactionById", () => {
        it("should fetch a transaction by id", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "success",
                totalSteps: 3,
                currentStep: 3,
                trajectory: null,
                error: null,
            };

            const id = await AddTransactionToDb(db, transaction);
            const retrieved = await FetchTransactionById(db, id);

            expect(retrieved.id).toBe(id);
            expect(retrieved.fromChainId).toBe("cosmoshub-4");
        });

        it("should reject when transaction not found", async () => {
            expect(FetchTransactionById(db, 999)).rejects.toThrow(
                "Transaction with id 999 not found"
            );
        });
    });

    describe("LoadAllTransactions", () => {
        it("should load all transactions", async () => {
            const tx1: Omit<TransactionRecord, "id"> = {
                timestamp: "2026-01-09T12:00:00Z",
                fromChainId: "chain1",
                fromChainAddress: "addr1",
                toChainId: "chain2",
                toChainAddress: "addr2",
                tokenIn: "TOKEN1",
                tokenOut: "TOKEN2",
                amountIn: "100",
                amountOut: "200",
                typeOfTransfer: "manual",
                status: "success",
                totalSteps: 1,
                currentStep: 1,
                trajectory: null,
                error: null,
            };

            const tx2: Omit<TransactionRecord, "id"> = {
                ...tx1,
                timestamp: "2026-01-09T13:00:00Z",
                fromChainId: "chain3",
            };

            await AddTransactionToDb(db, tx1);
            await AddTransactionToDb(db, tx2);

            const all = await LoadAllTransactions(db);
            expect(all.length).toBe(2);
        });

        it("should return transactions sorted by timestamp (newest first)", async () => {
            const old: Omit<TransactionRecord, "id"> = {
                timestamp: "2026-01-01T10:00:00Z",
                fromChainId: "chain1",
                fromChainAddress: "addr1",
                toChainId: "chain2",
                toChainAddress: "addr2",
                tokenIn: "TOKEN1",
                tokenOut: "TOKEN2",
                amountIn: "100",
                amountOut: "200",
                typeOfTransfer: "manual",
                status: "success",
                totalSteps: 1,
                currentStep: 1,
                trajectory: null,
                error: null,
            };

            const newer: Omit<TransactionRecord, "id"> = {
                ...old,
                timestamp: "2026-01-09T10:00:00Z",
            };

            await AddTransactionToDb(db, old);
            await AddTransactionToDb(db, newer);

            const all = await LoadAllTransactions(db);
            expect(all[0].timestamp).toBe("2026-01-09T10:00:00Z");
            expect(all[1].timestamp).toBe("2026-01-01T10:00:00Z");
        });

        it("should return empty array when no transactions exist", async () => {
            const all = await LoadAllTransactions(db);
            expect(all).toEqual([]);
        });
    });

    describe("UpdateTransactionStatus", () => {
        it("should update transaction status", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "in-progress",
                totalSteps: 3,
                currentStep: 1,
                trajectory: null,
                error: null,
            };

            const id = await AddTransactionToDb(db, transaction);
            await UpdateTransactionStatus(db, {
                id,
                status: "success",
                currentStep: 3,
            });

            const updated = await FetchTransactionById(db, id);
            expect(updated.status).toBe("success");
            expect(updated.currentStep).toBe(3);
            expect(updated.fromChainId).toBe("cosmoshub-4"); // Other fields unchanged
        });

        it("should update only specified fields", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "in-progress",
                totalSteps: 3,
                currentStep: 1,
                trajectory: null,
                error: null,
            };

            const id = await AddTransactionToDb(db, transaction);
            await UpdateTransactionStatus(db, {
                id,
                currentStep: 2,
            });

            const updated = await FetchTransactionById(db, id);
            expect(updated.currentStep).toBe(2);
            expect(updated.status).toBe("in-progress"); // Unchanged
        });

        it("should set error message", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "in-progress",
                totalSteps: 3,
                currentStep: 1,
                trajectory: null,
                error: null,
            };

            const id = await AddTransactionToDb(db, transaction);
            await UpdateTransactionStatus(db, {
                id,
                status: "failed",
                error: "Insufficient funds",
            });

            const updated = await FetchTransactionById(db, id);
            expect(updated.status).toBe("failed");
            expect(updated.error).toBe("Insufficient funds");
        });

        it("should reject when transaction not found", async () => {
            expect(
                UpdateTransactionStatus(db, {
                    id: 999,
                    status: "success",
                })
            ).rejects.toThrow("Transaction with id 999 not found");
        });
    });

    describe("UpdateTransactionInDb", () => {
        it("should update entire transaction", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: "2026-01-09T12:00:00Z",
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "in-progress",
                totalSteps: 3,
                currentStep: 1,
                trajectory: null,
                error: null,
            };

            const id = await AddTransactionToDb(db, transaction);
            const updatedTransaction: TransactionRecord = {
                id,
                timestamp: "2026-01-09T12:00:00Z",
                fromChainId: "juno-1",
                fromChainAddress: "juno1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "JUNO",
                tokenOut: "OSMO",
                amountIn: "200",
                amountOut: "1000",
                typeOfTransfer: "manual",
                status: "success",
                totalSteps: 1,
                currentStep: 1,
                trajectory: ["juno-1", "osmosis-1"],
                error: null,
            };

            await UpdateTransactionInDb(db, updatedTransaction);
            const retrieved = await FetchTransactionById(db, id);

            expect(retrieved).toMatchObject(updatedTransaction);
        });

        it("should reject when id is missing", async () => {
            const transaction = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart" as const,
                status: "in-progress" as const,
                totalSteps: 3,
                currentStep: 1,
                trajectory: null,
                error: null,
            };

            expect(UpdateTransactionInDb(db, transaction as TransactionRecord)).rejects.toThrow(
                "Transaction must have an id to be updated"
            );
        });
    });

    describe("DeleteTransactionById", () => {
        it("should delete a transaction", async () => {
            const transaction: Omit<TransactionRecord, "id"> = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "success",
                totalSteps: 3,
                currentStep: 3,
                trajectory: null,
                error: null,
            };

            const id = await AddTransactionToDb(db, transaction);
            await DeleteTransactionById(db, id);

            expect(FetchTransactionById(db, id)).rejects.toThrow(
                `Transaction with id ${id} not found`
            );
        });

        it("should not affect other transactions", async () => {
            const tx1: Omit<TransactionRecord, "id"> = {
                timestamp: "2026-01-09T10:00:00Z",
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "success",
                totalSteps: 3,
                currentStep: 3,
                trajectory: null,
                error: null,
            };

            const tx2: Omit<TransactionRecord, "id"> = {
                ...tx1,
                timestamp: "2026-01-09T11:00:00Z",
            };

            const tx3: Omit<TransactionRecord, "id"> = {
                ...tx1,
                timestamp: "2026-01-09T12:00:00Z",
            };

            const id1 = await AddTransactionToDb(db, tx1);
            const id2 = await AddTransactionToDb(db, tx2);
            const id3 = await AddTransactionToDb(db, tx3);

            await DeleteTransactionById(db, id2);

            const all = await LoadAllTransactions(db);
            expect(all.length).toBe(2);
            // Should be sorted by timestamp, newest first
            expect(all.map((t) => t.id)).toEqual([id3, id1]);
        });
    });

    describe("ClearAllTransactions", () => {
        it("should clear all transactions", async () => {
            const tx: Omit<TransactionRecord, "id"> = {
                timestamp: new Date().toISOString(),
                fromChainId: "cosmoshub-4",
                fromChainAddress: "cosmos1...",
                toChainId: "osmosis-1",
                toChainAddress: "osmo1...",
                tokenIn: "ATOM",
                tokenOut: "OSMO",
                amountIn: "100",
                amountOut: "500",
                typeOfTransfer: "smart",
                status: "success",
                totalSteps: 3,
                currentStep: 3,
                trajectory: null,
                error: null,
            };

            await AddTransactionToDb(db, tx);
            await AddTransactionToDb(db, tx);
            await AddTransactionToDb(db, tx);

            await ClearAllTransactions(db);

            const all = await LoadAllTransactions(db);
            expect(all).toEqual([]);
        });
    });

    describe("Transaction cleanup", () => {
        it("should keep only 50 most recent transactions", async () => {
            // Add 55 transactions
            const promises = [];
            for (let i = 0; i < 55; i++) {
                const tx: Omit<TransactionRecord, "id"> = {
                    timestamp: new Date(2026, 0, 1 + i).toISOString(),
                    fromChainId: "cosmoshub-4",
                    fromChainAddress: "cosmos1...",
                    toChainId: "osmosis-1",
                    toChainAddress: "osmo1...",
                    tokenIn: "ATOM",
                    tokenOut: "OSMO",
                    amountIn: "100",
                    amountOut: "500",
                    typeOfTransfer: "smart",
                    status: "success",
                    totalSteps: 3,
                    currentStep: 3,
                    trajectory: null,
                    error: null,
                };
                promises.push(AddTransactionToDb(db, tx));
            }
            await Promise.all(promises);

            // Close and reopen database to trigger cleanup
            db.close();
            db = await ConnectToDb();

            // Wait a bit for cleanup to complete
            await new Promise((resolve) => setTimeout(resolve, 100));

            const all = await LoadAllTransactions(db);
            expect(all.length).toBeLessThanOrEqual(50);
        }, 10000); // Increase timeout for this test
    });
});

