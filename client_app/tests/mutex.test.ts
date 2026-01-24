import { expect, test } from "bun:test";
import { Mutex } from "@/lib/mutex/mutex";

test("Should acquire lock immediately when not locked", async () => {
	const mutex = new Mutex();
	const startTime = Date.now();

	await mutex.lock();

	const endTime = Date.now();
	const duration = endTime - startTime;

	// Lock should be acquired almost immediately (within 10ms)
	expect(duration).toBeLessThan(10);
});

test("Should queue subsequent lock requests when already locked", async () => {
	const mutex = new Mutex();
	const executionOrder: number[] = [];

	// First task acquires the lock
	await mutex.lock();
	executionOrder.push(1);

	// Second task should wait
	const secondTaskPromise = (async () => {
		await mutex.lock();
		executionOrder.push(2);
	})();

	// Give the second task time to queue up
	await new Promise((resolve) => setTimeout(resolve, 50));

	// At this point, second task should still be waiting
	expect(executionOrder).toEqual([1]);

	// Release the lock
	mutex.unlock();

	// Wait for second task to complete
	await secondTaskPromise;

	// Now second task should have executed
	expect(executionOrder).toEqual([1, 2]);
});

test("Should release lock to next waiting task", async () => {
	const mutex = new Mutex();
	const executionOrder: number[] = [];

	await mutex.lock();
	executionOrder.push(1);

	// Queue two waiting tasks
	const secondTaskPromise = (async () => {
		await mutex.lock();
		executionOrder.push(2);
		mutex.unlock();
	})();

	const thirdTaskPromise = (async () => {
		await mutex.lock();
		executionOrder.push(3);
		mutex.unlock();
	})();

	// Give tasks time to queue
	await new Promise((resolve) => setTimeout(resolve, 50));

	// Release lock to first waiting task
	mutex.unlock();

	// Wait for both tasks to complete
	await secondTaskPromise;
	await thirdTaskPromise;

	// All tasks should execute in order
	expect(executionOrder).toEqual([1, 2, 3]);
});

test("Should fully unlock when no tasks are waiting", async () => {
	const mutex = new Mutex();

	// Acquire lock
	await mutex.lock();

	// Release lock with no waiting tasks
	mutex.unlock();

	// Should be able to acquire lock again immediately
	const startTime = Date.now();
	await mutex.lock();
	const endTime = Date.now();

	expect(endTime - startTime).toBeLessThan(10);
});

test("Should handle multiple sequential lock/unlock cycles", async () => {
	const mutex = new Mutex();
	const executionOrder: number[] = [];

	// Simulate 5 sequential tasks
	for (let i = 1; i <= 5; i++) {
		await mutex.lock();
		executionOrder.push(i);
		mutex.unlock();
	}

	// All tasks should execute in order
	expect(executionOrder).toEqual([1, 2, 3, 4, 5]);
});

test("Should handle concurrent lock requests from multiple tasks", async () => {
	const mutex = new Mutex();
	const executionOrder: number[] = [];

	// Start first task
	const task1 = (async () => {
		await mutex.lock();
		executionOrder.push(1);
		await new Promise((resolve) => setTimeout(resolve, 30));
		mutex.unlock();
	})();

	// Give task1 time to acquire lock
	await new Promise((resolve) => setTimeout(resolve, 10));

	// Start remaining tasks
	const task2 = (async () => {
		await mutex.lock();
		executionOrder.push(2);
		mutex.unlock();
	})();

	const task3 = (async () => {
		await mutex.lock();
		executionOrder.push(3);
		mutex.unlock();
	})();

	const task4 = (async () => {
		await mutex.lock();
		executionOrder.push(4);
		mutex.unlock();
	})();

	// Wait for all tasks to complete
	await Promise.all([task1, task2, task3, task4]);

	// First task should execute first, then others in queue order
	expect(executionOrder[0]).toBe(1);
	expect(executionOrder.length).toBe(4);
	expect(new Set(executionOrder)).toEqual(new Set([1, 2, 3, 4]));
});

test("Should prevent race conditions in critical sections", async () => {
	const mutex = new Mutex();
	let sharedCounter = 0;

	// Simulate 10 concurrent increments with mutex protection
	const tasks = Array.from({ length: 10 }, async (_, _i) => {
		await mutex.lock();
		const currentValue = sharedCounter;
		// Simulate some async work
		await new Promise((resolve) => setTimeout(resolve, 5));
		sharedCounter = currentValue + 1;
		mutex.unlock();
	});

	await Promise.all(tasks);

	// Counter should be exactly 10 (no race condition)
	expect(sharedCounter).toBe(10);
});

test("Should handle lock/unlock with no intermediate operations", async () => {
	const mutex = new Mutex();

	// Rapid lock/unlock cycles
	for (let i = 0; i < 100; i++) {
		await mutex.lock();
		mutex.unlock();
	}

	// Should be able to acquire lock one more time
	await mutex.lock();
	mutex.unlock();

	// If we got here without deadlock, test passes
	expect(true).toBe(true);
});
