"use client";

import { createContext, type ReactNode, useContext, useState } from "react";
import { v7 as uuidv7 } from "uuid";
import { Mutex } from "../lib/mutex/mutex";

const TaskTypes = {
    // IBC transfers
    IBC_TRANSFER: "ibc-basic-transfer",
    IBC_HOOKS_TRANSFER: "ibc-hooks-transfer",
    // Osmosis swaps
    OSMO_SWAP: "osmosis-swap",
    OSMO_SPLIT_ROUTE_SWAP: "osmosis-split-route-swap",
    // Indexed DB related tasks
    INSERT_TRANSACTION: "insert-transaction",
    UPDATE_TRANSACTION: "update-transaction",
} as const;

export type Task = {
    // id is a unique identifier for the task
    id: string;
    // type is the type of the task
    type: (typeof TaskTypes)[keyof typeof TaskTypes];
    // data is a map of key-value pairs that contains the data for the task
    data: Map<string, string | number | boolean | object>;
    // boolean value to determin if the task has been executed
    done?: boolean;
};

/**
 * A synchronous task provider context
 * @interface TaskContextType
 * @property {Map<string, Task>} tasks - A map of tasks
 * @property {function} addTask - Add a new task
 * @property {function} getTaskById - Get a task by id
 * @property {function} getTasksByType - Get tasks by type
 * @property {function} getAllTasks - Get all tasks
 * @property {function} updateTaskById - Update a task by id
 * @property {function} deleteTaskById - Delete a task by id
 * @property {function} deleteTasksByType - Delete tasks by type
 * @property {function} deleteTasksByDone - Delete tasks by done
 * @property {function} deleteAllTasks - Delete all tasks
 */
interface TaskContextType {
    // state
    tasks: Map<string, Task>;

    // addition
    addTask: (task: Task) => Promise<void>;

    // information
    getTaskById: (id: string) => Promise<Task | undefined>;
    getTasksByType: (type: (typeof TaskTypes)[keyof typeof TaskTypes]) => Promise<Task[]>;
    getAllTasks: () => Promise<Task[]>;

    // updating
    updateTaskById: (
        id: string,
        data: Map<string, string | number | boolean | object>,
    ) => Promise<void>;

    // deletion
    deleteTaskById: (id: string) => Promise<void>;
    deleteTasksByType: (type: (typeof TaskTypes)[keyof typeof TaskTypes]) => Promise<void>;
    deleteTasksByDone: (done: boolean) => Promise<void>;
    deleteAllTasks: () => Promise<void>;
}

const TaskContext = createContext<TaskContextType | undefined>(undefined);

export const useTaskProvider = () => {
    const context = useContext(TaskContext);
    if (!context) {
        throw new Error("useTask must be used within TaskProvider");
    }
    return context;
};

export const TaskProvider = ({ children }: { children: ReactNode }) => {
    const [tasks, setTasks] = useState<Map<string, Task>>(new Map<string, Task>());
    const mutex = new Mutex();

    // add function to add a new task
    const addTask = async (task: Task) => {
        try {
            await mutex.lock();
            const uuid: string = uuidv7();
            const newTask: Task = {
                id: uuid,
                type: task.type,
                data: task.data,
                done: false,
            };
            tasks.set(uuid, newTask);
            setTasks(tasks);
        } finally {
            mutex.unlock();
        }
    };

    // get function to get a task by id
    const getTaskById = async (id: string) => {
        try {
            await mutex.lock();
            return tasks.get(id);
        } finally {
            mutex.unlock();
        }
    };

    // get function to get a task by type
    const getTasksByType = async (type: (typeof TaskTypes)[keyof typeof TaskTypes]) => {
        try {
            await mutex.lock();
            return Array.from(tasks.values()).filter((task) => task.type === type);
        } finally {
            mutex.unlock();
        }
    };

    // get function to get all tasks
    const getAllTasks = async () => {
        try {
            await mutex.lock();
            return Array.from(tasks.values());
        } finally {
            mutex.unlock();
        }
    };

    // update function to update a task by id
    const updateTaskById = async (
        id: string,
        data: Map<string, string | number | boolean | object>,
    ) => {
        try {
            await mutex.lock();
            const task = tasks.get(id);
            if (!task) {
                return;
            }
            task.data = data;
            tasks.set(id, task);
            setTasks(tasks);
        } finally {
            mutex.unlock();
        }
    };

    // delete function to delete a task by type
    const deleteTasksByType = async (type: (typeof TaskTypes)[keyof typeof TaskTypes]) => {
        try {
            await mutex.lock();
            for (const [id, task] of tasks.entries()) {
                if (task.type === type) {
                    tasks.delete(id);
                }
            }
            setTasks(tasks);
        } finally {
            mutex.unlock();
        }
    };

    // delete function to delete a task by done
    const deleteTasksByDone = async (done: boolean) => {
        try {
            await mutex.lock();
            for (const [id, task] of tasks.entries()) {
                if (task.done === done) {
                    tasks.delete(id);
                }
            }
            setTasks(tasks);
        } finally {
            mutex.unlock();
        }
    };

    // delete function to delete all tasks
    const deleteAllTasks = async () => {
        try {
            await mutex.lock();
            tasks.clear();
            setTasks(tasks);
        } finally {
            mutex.unlock();
        }
    };

    // delete function to delete a task by id
    const deleteTaskById = async (id: string) => {
        try {
            await mutex.lock();
            tasks.delete(id);
            setTasks(tasks);
        } finally {
            mutex.unlock();
        }
    };

    return (
        <TaskContext.Provider
            value={{
                tasks,
                addTask,
                getTaskById,
                getTasksByType,
                getAllTasks,
                updateTaskById,
                deleteTasksByType,
                deleteTasksByDone,
                deleteAllTasks,
                deleteTaskById,
            }}
        >
            {children}
        </TaskContext.Provider>
    );
};
