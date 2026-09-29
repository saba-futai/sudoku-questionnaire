/** Mirrors the Go DTOs in backend/internal/{schema,services,handlers}. */

export type QuestionType =
  | 'single'
  | 'multi'
  | 'select'
  | 'text'
  | 'textarea'
  | 'rating'
  | 'date'
  | 'image';

export interface Option {
  value: string;
  label: string;
}

export interface Question {
  key: string;
  type: QuestionType;
  label: string;
  help?: string;
  required: boolean;
  options?: Option[];
  minLength?: number;
  maxLength?: number;
  min?: number;
  max?: number;
  maxImages?: number;
}

export interface Step {
  id: string;
  title: string;
  description?: string;
  questions: Question[];
}

export interface Definition {
  title: string;
  description?: string;
  steps: Step[];
}

export type QuestionnaireStatus = 'draft' | 'published' | 'closed';

export interface QuestionnaireSummary {
  id: number;
  slug: string;
  status: QuestionnaireStatus;
  version: number;
  layout: string;
  title: string;
  description: string;
  stepCount: number;
  questionCount: number;
  responseCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface QuestionnaireDetail {
  id: number;
  slug: string;
  status: QuestionnaireStatus;
  version: number;
  layout: string;
  createdAt: string;
  updatedAt: string;
  definition?: Definition;
  undecodable?: boolean;
  error?: string;
}

/** What the public fill endpoint returns: ciphertext, never plaintext. */
export interface PublicQuestionnaire {
  slug: string;
  version: number;
  layout: string;
  derivedKey: string;
  fingerprint: string;
  schemaCipher: string;
}

export interface PublicQuestionnaireSummary {
  slug: string;
  title: string;
  description: string;
  stepCount: number;
  questionCount: number;
}

export type AnswerValue = string | string[] | number | number[] | null;

export interface ImageView {
  id: number;
  fieldKey: string;
  mime: string;
  width: number;
  height: number;
  sizeBytes: number;
  url: string;
}

export interface ResponseView {
  id: number;
  questionnaireId: number;
  answers: Record<string, unknown>;
  images: ImageView[];
  durationMs: number;
  index: number;
  createdAt: string;
  updatedAt: string;
}

export interface OptionStat {
  value: string;
  label: string;
  count: number;
  percent: number;
}

export interface QuestionStat {
  key: string;
  type: QuestionType;
  label: string;
  answered: number;
  skipped: number;
  options?: OptionStat[];
  average?: number;
  sum?: number;
  samples?: string[];
  sampleTotal?: number;
  images?: ImageView[];
  longestChars?: number;
}

export interface TimelinePoint {
  date: string;
  count: number;
}

export interface StatsView {
  questionnaireId: number;
  title: string;
  total: number;
  avgDurationMs: number;
  firstAt: string | null;
  lastAt: string | null;
  questions: QuestionStat[];
  timeline: TimelinePoint[];
  totalImages: number;
}

export interface OverviewCounts {
  questionnaires: number;
  published: number;
  draft: number;
  closed: number;
  responses: number;
  images: number;
  responsesToday: number;
}

export interface LeaderboardEntry {
  id: number;
  slug: string;
  title: string;
  count: number;
}

export interface AuditLog {
  id: number;
  actor: string;
  action: string;
  targetType: string;
  targetId: number;
  detail: string;
  createdAt: string;
}

export interface DecodeReport {
  plaintext: string;
  clueSymbols: number;
  paddingBytes: number;
  rejectedByte: number;
  plaintextLen: number;
  checksumOk: boolean;
  trailerHex: string;
}

export interface RawResponse {
  id: number;
  cipher: string;
  layout: string;
  length: number;
  inspection: DecodeReport;
}

export interface CodecInfo {
  questionnaireId: number;
  slug: string;
  layout: string;
  fingerprint: string;
  derivedKey: string;
  minEncodings: number;
  maxEncodings: number;
  distinctPuzzles: number;
  gridCount: number;
  posSetCount: number;
  gridsSha256: string;
}

export interface LoginResult {
  token: string;
  username: string;
  expiresAt: string;
}
