// Single shared instance of the pocketknife-generated typed client.
//
// baseUrl is "" on purpose: the UI is served from the same origin as the API
// (pocketknife answers both /ui/workouts/... and /apps/workouts/... ), so an
// origin-absolute path needs no host and no CORS. The generated client is the
// only thing that knows the URL scheme, query syntax and error envelope — we
// never hand-write a fetch against the API here.
import { WorkoutsClient } from "./client";

export const api = new WorkoutsClient();

export {
  ApiError,
  type Workout,
  type WorkoutCreateInput,
  type WorkoutUpdateInput,
  type WorkoutType,
  type Exercise,
  type ExerciseCreateInput,
  type ExerciseUpdateInput,
  type ExerciseKind,
  type ExerciseStation,
} from "./client";
