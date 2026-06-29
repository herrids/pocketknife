import { EspressoTrackerClient } from "@/client";

// One shared client instance for the whole app. Defaults to same-origin
// (see ClientOptions in ./client) — every read and write goes through this.
export const client = new EspressoTrackerClient();
