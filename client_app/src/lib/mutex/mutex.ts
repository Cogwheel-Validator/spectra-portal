export class Mutex {
    private isLocked: boolean = false;
    // While array is ususally not recommended for this it should be okay
    // i expect max 10 tasks to actually happen at best case
    private queue: Array<() => void> = [];

    public async lock(): Promise<void> {
        // If already locked, wait in queue (add to queue)
        if (this.isLocked) {
            await new Promise<void>((resolve) => {
                this.queue.push(resolve);
            });
        }
        // Acquire the lock
        this.isLocked = true;
    }

    public unlock(): void {
        // If someone is waiting, give them the lock
        if (this.queue.length > 0) {
            const resolve = this.queue.shift();
            if (resolve) {
                resolve(); // This should let the next waiting task acquire the lock
            }
        } else {
            // No one waiting, just unlock
            this.isLocked = false;
        }
    }
}
