/*
 * Derived-data helpers shared by the dashboard pages. Everything here is pure so the pages
 * stay declarative: a page maps a payload into series, it never computes a statistic twice.
 */

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

export function sum(values: number[]): number {
	return values.reduce((total, v) => total + (Number.isFinite(v) ? v : 0), 0);
}

/*
 * A delta is only honest when there is a comparable stretch behind it: split the buckets in
 * two equal halves and compare the recent one to the one before it. Fewer than two buckets,
 * or a previous half with nothing in it, has no percentage to report — the caller shows no
 * delta rather than a fabricated one.
 */
export function periodDelta(values: number[], unit: string): string | undefined {
	const half = Math.floor(values.length / 2);
	if (half < 1) return undefined;
	const previous = sum(values.slice(values.length - 2 * half, values.length - half));
	const current = sum(values.slice(values.length - half));
	if (previous <= 0) return undefined;
	const pct = Math.round(((current - previous) / previous) * 100);
	return `${pct >= 0 ? '+' : ''}${pct}% vs previous ${half} ${unit}${half === 1 ? '' : 's'}`;
}

/*
 * The timeline arrives as one aligned array per series, so page totals are column sums and
 * "how many machines worked that day" is a column count of non-zero cells.
 */
export function columnTotals(rows: number[][]): number[] {
	const width = rows.reduce((max, r) => Math.max(max, r.length), 0);
	const out = new Array(width).fill(0);
	for (const row of rows) {
		for (let i = 0; i < row.length; i++) out[i] += Number.isFinite(row[i]) ? row[i] : 0;
	}
	return out;
}

export function columnActive(rows: number[][]): number[] {
	const width = rows.reduce((max, r) => Math.max(max, r.length), 0);
	const out = new Array(width).fill(0);
	for (const row of rows) {
		for (let i = 0; i < row.length; i++) if (row[i] > 0) out[i] += 1;
	}
	return out;
}

export function hours(seconds: number): number {
	return Math.round((seconds / 3600) * 10) / 10;
}

export function formatDuration(seconds: number): string {
	const h = Math.floor(seconds / 3600);
	const m = Math.round((seconds % 3600) / 60);
	if (h > 0) return `${h}h${String(m).padStart(2, '0')}m`;
	return `${m}m`;
}

export function formatTokens(n: number): string {
	if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
	return `${n}`;
}

export function formatBytes(n: number): string {
	if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(1)} MB`;
	if (n >= 1024) return `${Math.round(n / 1024)} kB`;
	return `${n} B`;
}

export function formatAge(iso: string): string {
	const then = new Date(iso).getTime();
	if (!iso || isNaN(then)) return '—';
	const s = Math.max(0, Math.floor((Date.now() - then) / 1000));
	if (s < 60) return 'just now';
	if (s < 3600) return `${Math.floor(s / 60)}m ago`;
	if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
	return `${Math.floor(s / 86400)}d ago`;
}

export function formatSpan(seconds: number): string {
	if (!Number.isFinite(seconds) || seconds <= 0) return 'now';
	const h = Math.floor(seconds / 3600);
	const m = Math.floor((seconds % 3600) / 60);
	if (h >= 24) return `${Math.floor(h / 24)}d ${h % 24}h`;
	if (h > 0) return `${h}h ${m}m`;
	return `${Math.max(1, m)}m`;
}

/*
 * Fallback only: the server computes the countdown against its own clock, which is the clock
 * the reset was recorded against. This recomputes it locally when it did not come through.
 */
export function formatCountdown(iso: string): string {
	const target = new Date(iso).getTime();
	if (!iso || isNaN(target)) return 'unknown';
	return formatSpan(Math.floor((target - Date.now()) / 1000));
}

export function dayKey(d: Date): string {
	const month = String(d.getUTCMonth() + 1).padStart(2, '0');
	const day = String(d.getUTCDate()).padStart(2, '0');
	return `${d.getUTCFullYear()}-${month}-${day}`;
}

/*
 * Gap-filling is the point: bucketing timestamps alone leaves holes on quiet days, and a
 * line chart with holes reads as a shorter, busier history than the one that happened.
 */
export function dayWindow(days: number, end = new Date()): string[] {
	const labels: string[] = [];
	const cursor = Date.UTC(end.getUTCFullYear(), end.getUTCMonth(), end.getUTCDate());
	for (let i = days - 1; i >= 0; i--) labels.push(dayKey(new Date(cursor - i * 86_400_000)));
	return labels;
}

export function bucketByDay(
	entries: { iso: string; weight?: number }[],
	labels: string[]
): number[] {
	const index = new Map(labels.map((l, i) => [l, i]));
	const out = new Array(labels.length).fill(0);
	for (const entry of entries) {
		const d = new Date(entry.iso);
		if (isNaN(d.getTime())) continue;
		const i = index.get(dayKey(d));
		if (i === undefined) continue;
		out[i] += entry.weight ?? 1;
	}
	return out;
}

/*
 * `YYYY-MM-DD` and `YYYY-MM` both come back from the timeline endpoint; the axis wants the
 * short human form of whichever it got.
 */
export function bucketLabel(label: string): string {
	const parts = label.split('-');
	const month = MONTHS[Number(parts[1]) - 1] ?? label;
	if (parts.length >= 3) return `${month} ${Number(parts[2])}`;
	return `${month} ${parts[0]?.slice(2) ?? ''}`;
}
