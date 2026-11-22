import type { HelpTopic } from "./help";
import { RECRUITING_TOPICS } from "./topicsRecruiting";
import { SETTINGS_TOPICS } from "./topicsSettings";
import { TOOL_TOPICS } from "./topicsTools";

/** Every help topic, in the order the help page lists them (Getting started first). */
export const HELP_TOPICS: HelpTopic[] = [...RECRUITING_TOPICS, ...TOOL_TOPICS, ...SETTINGS_TOPICS];
