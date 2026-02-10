/**
 * Purpose of this module is to have a centralized managing system where the local history of
 * every transaction is available on that users device using IndexedDB.
 */
"use client";
import logger from "@/lib/clientLogger";

// Database version and name
// in case you need to update something in the database, you can increment the version
const DB_VERSION: number = 1.2;
const DB_NAME: string = "spectra_ibc";
const TRANSACTION_STORE = "transactions";
const MAX_TRANSACTIONS = 50;

export type TransactionRecord = {
    id?: number; // Optional because auto-incremented
    timestamp: string;
    fromChainId: string;
    fromChainAddress: string;
    toChainId: string;
    toChainAddress: string;
    tokenIn: string;
    tokenOut: string;
    amountIn: string;
    amountOut: string;
    typeOfTransfer: "manual" | "smart";
    status: "success" | "failed" | "in-progress" | "canceled";
    totalSteps: number;
    currentStep: number;
    // trajectory as in are there any chainIds in between the fromChainId and the toChainId
    trajectory: string[] | null;
    error: string | null;
    // confirmation weather the transaction involves a swap on the broker chain
    swapInvolved: boolean;
    /** Multi-hop tracking: 0–100 percentage as each chain confirms (smart/wasm/PFM single-step transfers) */
    multiHopProgress?: number | null;
    /** Total chains in the path for multi-hop (e.g. 4 → 25% per chain) */
    multiHopTotalChains?: number | null;
};

export type TransactionUpdate = {
    id: number;
    status?: "success" | "failed" | "in-progress" | "canceled";
    currentStep?: number;
    error?: string | null;
    swapInvolved?: boolean;
    multiHopProgress?: number | null;
    multiHopTotalChains?: number | null;
};

/**
 * Opens a connection to the IndexedDB database.
 * Returns a Promise that resolves with the database connection.
 */
export function ConnectToDb(): Promise<IDBDatabase> {
    return new Promise((resolve, reject) => {
        const request = indexedDB.open(DB_NAME, DB_VERSION);

        request.onerror = () => {
            logger.error("Failed to open database", request.error);
            reject(new Error("Failed to open database"));
        };

        request.onupgradeneeded = (event) => {
            const db = (event.target as IDBOpenDBRequest).result;
            logger.info("Database upgrade needed, creating object stores");

            // Create object store with auto-incrementing id
            if (!db.objectStoreNames.contains(TRANSACTION_STORE)) {
                const objectStore = db.createObjectStore(TRANSACTION_STORE, {
                    keyPath: "id",
                    autoIncrement: true,
                });
                // Create index on timestamp for efficient sorting/querying
                objectStore.createIndex("timestamp", "timestamp", { unique: false });
                objectStore.createIndex("status", "status", { unique: false });
                logger.info("Created transactions object store");
            }
        };

        request.onsuccess = () => {
            const db = request.result;
            logger.info("Database opened successfully");

            // Handle version changes (e.g., another tab updated the database)
            db.onversionchange = () => {
                db.close();
                logger.warn("Database version changed, reloading page");
                window.location.reload();
            };

            // Clean up old transactions if needed
            cleanupOldTransactions(db).catch((err) =>
                logger.error("Failed to cleanup old transactions", err),
            );

            resolve(db);
        };
    });
}

/**
 * Removes oldest transactions if there are more than MAX_TRANSACTIONS
 */
async function cleanupOldTransactions(db: IDBDatabase): Promise<void> {
    return new Promise((resolve, reject) => {
        const transaction = db.transaction(TRANSACTION_STORE, "readwrite");
        const store = transaction.objectStore(TRANSACTION_STORE);
        const request = store.getAll();

        request.onsuccess = () => {
            const transactions = request.result as TransactionRecord[];
            if (transactions.length > MAX_TRANSACTIONS) {
                logger.info(
                    `Found ${transactions.length} transactions, deleting oldest ${
                        transactions.length - MAX_TRANSACTIONS
                    }`,
                );
                // Sort by timestamp (oldest first)
                transactions.sort(
                    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
                );
                // Delete oldest transactions
                const toDelete = transactions.slice(0, transactions.length - MAX_TRANSACTIONS);
                toDelete.forEach((t) => {
                    if (t.id !== undefined) {
                        store.delete(t.id);
                    }
                });
            }
            resolve();
        };

        request.onerror = () => {
            logger.error("Failed to cleanup transactions", request.error);
            reject(new Error("Failed to cleanup transactions"));
        };
    });
}

/**
 * Adds a new transaction to the database.
 * Returns a Promise that resolves with the generated ID.
 */
export function AddTransactionToDb(
    db: IDBDatabase,
    transaction: Omit<TransactionRecord, "id">,
): Promise<number> {
    return new Promise((resolve, reject) => {
        const tx = db.transaction(TRANSACTION_STORE, "readwrite");
        const store = tx.objectStore(TRANSACTION_STORE);
        const request = store.add(transaction);

        request.onsuccess = () => {
            const id = request.result as number;
            logger.info("Transaction added successfully with id:", id);
            resolve(id);
        };

        request.onerror = () => {
            logger.error("Failed to add transaction", request.error);
            reject(new Error("Failed to add transaction"));
        };
    });
}

/**
 * Updates specific fields of a transaction (status, currentStep, error).
 * More efficient than updating the entire record.
 */
export function UpdateTransactionStatus(db: IDBDatabase, update: TransactionUpdate): Promise<void> {
    return new Promise((resolve, reject) => {
        const tx = db.transaction(TRANSACTION_STORE, "readwrite");
        const store = tx.objectStore(TRANSACTION_STORE);
        const getRequest = store.get(update.id);

        getRequest.onsuccess = () => {
            const transaction = getRequest.result as TransactionRecord | undefined;
            if (!transaction) {
                reject(new Error(`Transaction with id ${update.id} not found`));
                return;
            }

            // Update only the specified fields
            if (update.status !== undefined) {
                transaction.status = update.status;
            }
            if (update.currentStep !== undefined) {
                transaction.currentStep = update.currentStep;
            }
            if (update.error !== undefined) {
                transaction.error = update.error;
            }
            if (update.swapInvolved !== undefined) {
                transaction.swapInvolved = update.swapInvolved;
            }
            if (update.multiHopProgress !== undefined) {
                transaction.multiHopProgress = update.multiHopProgress;
            }
            if (update.multiHopTotalChains !== undefined) {
                transaction.multiHopTotalChains = update.multiHopTotalChains;
            }

            const putRequest = store.put(transaction);
            putRequest.onsuccess = () => {
                logger.info("Transaction updated successfully");
                resolve();
            };
            putRequest.onerror = () => {
                logger.error("Failed to update transaction", putRequest.error);
                reject(new Error("Failed to update transaction"));
            };
        };

        getRequest.onerror = () => {
            logger.error("Failed to fetch transaction for update", getRequest.error);
            reject(new Error("Failed to fetch transaction"));
        };
    });
}

/**
 * Updates an entire transaction record.
 * Use UpdateTransactionStatus for partial updates.
 */
export function UpdateTransactionInDb(
    db: IDBDatabase,
    transaction: TransactionRecord,
): Promise<void> {
    return new Promise((resolve, reject) => {
        if (!transaction.id) {
            reject(new Error("Transaction must have an id to be updated"));
            return;
        }

        const tx = db.transaction(TRANSACTION_STORE, "readwrite");
        const store = tx.objectStore(TRANSACTION_STORE);
        const request = store.put(transaction);

        request.onsuccess = () => {
            logger.info("Transaction updated successfully");
            resolve();
        };

        request.onerror = () => {
            logger.error("Failed to update transaction", request.error);
            reject(new Error("Failed to update transaction"));
        };
    });
}

/**
 * Loads all transactions from the database.
 * Returns transactions sorted by timestamp (newest first).
 */
export function LoadAllTransactions(db: IDBDatabase): Promise<TransactionRecord[]> {
    return new Promise((resolve, reject) => {
        const tx = db.transaction(TRANSACTION_STORE, "readonly");
        const store = tx.objectStore(TRANSACTION_STORE);
        const request = store.getAll();

        request.onsuccess = () => {
            const transactions = request.result as TransactionRecord[];
            // Sort by timestamp, newest first
            transactions.sort(
                (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime(),
            );
            resolve(transactions);
        };

        request.onerror = () => {
            logger.error("Failed to load transactions", request.error);
            reject(new Error("Failed to load transactions"));
        };
    });
}

/**
 * Fetches a single transaction by its ID.
 */
export function FetchTransactionById(db: IDBDatabase, id: number): Promise<TransactionRecord> {
    return new Promise((resolve, reject) => {
        const tx = db.transaction(TRANSACTION_STORE, "readonly");
        const store = tx.objectStore(TRANSACTION_STORE);
        const request = store.get(id);

        request.onsuccess = () => {
            const transaction = request.result as TransactionRecord | undefined;
            if (transaction) {
                resolve(transaction);
            } else {
                reject(new Error(`Transaction with id ${id} not found`));
            }
        };

        request.onerror = () => {
            logger.error("Failed to fetch transaction", request.error);
            reject(new Error("Failed to fetch transaction"));
        };
    });
}

/**
 * Deletes a transaction by its ID.
 */
export function DeleteTransactionById(db: IDBDatabase, id: number): Promise<void> {
    return new Promise((resolve, reject) => {
        const tx = db.transaction(TRANSACTION_STORE, "readwrite");
        const store = tx.objectStore(TRANSACTION_STORE);
        const request = store.delete(id);

        request.onsuccess = () => {
            logger.info("Transaction deleted successfully");
            resolve();
        };

        request.onerror = () => {
            logger.error("Failed to delete transaction", request.error);
            reject(new Error("Failed to delete transaction"));
        };
    });
}

/**
 * Clears all transactions from the database.
 */
export function ClearAllTransactions(db: IDBDatabase): Promise<void> {
    return new Promise((resolve, reject) => {
        const tx = db.transaction(TRANSACTION_STORE, "readwrite");
        const store = tx.objectStore(TRANSACTION_STORE);
        const request = store.clear();

        request.onsuccess = () => {
            logger.info("All transactions cleared");
            resolve();
        };

        request.onerror = () => {
            logger.error("Failed to clear transactions", request.error);
            reject(new Error("Failed to clear transactions"));
        };
    });
}

/**
 * Completely deletes the database.
 * Use during development to reset everything.
 * Call this from browser console: window.deleteSpectraDb()
 */
export function DeleteDatabase(): Promise<void> {
    return new Promise((resolve, reject) => {
        const request = indexedDB.deleteDatabase(DB_NAME);

        request.onsuccess = () => {
            logger.info("Database deleted successfully");
            resolve();
        };

        request.onerror = () => {
            logger.error("Failed to delete database", request.error);
            reject(new Error("Failed to delete database"));
        };

        request.onblocked = () => {
            logger.warn("Database deletion blocked. Close all tabs using this database.");
            reject(new Error("Database deletion blocked"));
        };
    });
}

// Make DeleteDatabase available globally for easy access during development
// DO NOT USE THIS UNLESS YOU REALLY KNOW WHAT YOU ARE DOING!
if (typeof window !== "undefined") {
    (window as typeof window & { deleteSpectraDb: typeof DeleteDatabase }).deleteSpectraDb =
        DeleteDatabase;
}
