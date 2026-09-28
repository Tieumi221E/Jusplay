// The V1 comment data as the page holds it (api/comments), and what
// filtering reports about it. The filtering itself is the window's
// (internal/filter, the capability comments.filter): the page draws the
// comments it keeps.

export interface V1Comment {
  id: string;
  no: number;
  vposMs: number;
  body: string;
  commands: string[];
  userId: string;
  isPremium: boolean;
  score: number;
  postedAt: string;
  nicoruCount: number;
  nicoruId: string | null;
  source: string;
  isMyPost: boolean;
  [extra: string]: unknown;
}

export interface V1Thread {
  id: unknown;
  fork: string;
  commentCount: number;
  comments: V1Comment[];
  [extra: string]: unknown;
}

export type Rule =
  | "fork" | "script" | "time" | "ngShare" | "user" | "word" | "command"
  | "naka" | "ue" | "shita" | "colored" | "big" | "small" | "ca" | "anonymous" | "length" | "cap";

export interface FilterStats {
  total: number;
  shown: number;
  byRule: Partial<Record<Rule, number>>;
  byFork: Record<string, { total: number; shown: number }>;
  /** NG word entries that are not valid regular expressions. */
  badPatterns: string[];
  /** The total budget in effect (0 = none) and how many were eligible for it. */
  cap: { limit: number; eligible: number };
}
