<script lang="ts">
	import { Badge, Button, Card, icons, toast } from '@facile/muse';
	import { marked, type Tokens, type Token } from 'marked';
	import MermaidBlock from './MermaidBlock.svelte';

	let { content = '' }: { content: string } = $props();

	function stripFrontmatter(raw: string): string {
		if (raw.startsWith('---\n') || raw.startsWith('---\r\n')) {
			const end = raw.indexOf('\n---', 4);
			if (end >= 0) {
				const cut = raw.indexOf('\n', end + 4);
				return cut >= 0 ? raw.slice(cut + 1) : '';
			}
		}
		return raw;
	}

	const cleanBody = $derived(stripFrontmatter(content));
	const tokens = $derived(marked.lexer(cleanBody) as Token[]);

	function parseBarChart(raw: string) {
		const lines = raw.trim().split('\n');
		let title = '';
		const items: { label: string; value: number; formatted: string }[] = [];

		for (const line of lines) {
			const trimmed = line.trim();
			if (!trimmed) continue;
			const colon = trimmed.indexOf(':');
			if (colon < 0) continue;
			const key = trimmed.slice(0, colon).trim();
			const rawVal = trimmed.slice(colon + 1).trim();
			if (key.toLowerCase() === 'title') {
				title = rawVal;
				continue;
			}
			const numMatch = rawVal.match(/[\d.,]+/);
			const num = numMatch ? parseFloat(numMatch[0].replace(',', '.')) : 0;
			items.push({ label: key, value: num, formatted: rawVal });
		}

		const max = Math.max(...items.map((i) => i.value), 1);
		return { title, items, max };
	}

	function parseMetrics(raw: string) {
		const blocks = raw.trim().split(/---+/);
		return blocks
			.map((block) => {
				const lines = block.trim().split('\n');
				let label = '';
				let val = '';
				let sub = '';
				for (const line of lines) {
					const colon = line.indexOf(':');
					if (colon < 0) continue;
					const k = line.slice(0, colon).trim().toLowerCase();
					const v = line.slice(colon + 1).trim();
					if (k === 'label' || k === 'title') label = v;
					else if (k === 'val' || k === 'value') val = v;
					else if (k === 'sub' || k === 'desc' || k === 'subtitle') sub = v;
				}
				if (!val && !label) return null;
				return { label, val, sub };
			})
			.filter((m): m is { label: string; val: string; sub: string } => m !== null);
	}

	interface AlertData {
		isAlert: boolean;
		tone: 'note' | 'tip' | 'important' | 'warning' | 'caution' | 'pros' | 'cons';
		title: string;
		body: string;
	}

	function parseAlert(raw: string): AlertData {
		const trimmed = raw.trim();
		const match = trimmed.match(/^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION|PROS|CONS|ADVANTAGE|DRAWBACK)\]\s*(.*)$/im);
		if (!match) {
			return { isAlert: false, tone: 'note', title: '', body: raw };
		}
		const rawType = match[1].toUpperCase();
		let tone: AlertData['tone'] = 'note';
		let defaultTitle = 'Note';

		if (rawType === 'TIP' || rawType === 'PROS' || rawType === 'ADVANTAGE') {
			tone = rawType === 'PROS' || rawType === 'ADVANTAGE' ? 'pros' : 'tip';
			defaultTitle = rawType === 'PROS' || rawType === 'ADVANTAGE' ? 'Advantages' : 'Tip';
		} else if (rawType === 'IMPORTANT') {
			tone = 'important';
			defaultTitle = 'Important';
		} else if (rawType === 'WARNING') {
			tone = 'warning';
			defaultTitle = 'Warning';
		} else if (rawType === 'CAUTION' || rawType === 'CONS' || rawType === 'DRAWBACK') {
			tone = rawType === 'CONS' || rawType === 'DRAWBACK' ? 'cons' : 'caution';
			defaultTitle = rawType === 'CONS' || rawType === 'DRAWBACK' ? 'Drawbacks' : 'Caution';
		}

		const firstLineExtra = match[2].trim();
		const afterMatch = trimmed.slice(match[0].length).trim();
		const body = firstLineExtra ? `${firstLineExtra}\n${afterMatch}` : afterMatch;

		return {
			isAlert: true,
			tone,
			title: defaultTitle,
			body: body || raw
		};
	}

	function parseListItem(text: string) {
		const trimmed = text.trim();
		if (
			trimmed.startsWith('[+]') ||
			trimmed.startsWith('+ ') ||
			trimmed.startsWith('✅') ||
			trimmed.toLowerCase().startsWith('[pro]') ||
			trimmed.toLowerCase().startsWith('[pros]') ||
			trimmed.toLowerCase().startsWith('[advantage]')
		) {
			const clean = trimmed.replace(/^(\[\+\]|\+\s+|✅|\[pro[s]?\]|\[advantage\])\s*/i, '');
			return { kind: 'pro', clean };
		}
		if (
			trimmed.startsWith('[-]') ||
			trimmed.startsWith('- ') ||
			trimmed.startsWith('❌') ||
			trimmed.toLowerCase().startsWith('[con]') ||
			trimmed.toLowerCase().startsWith('[cons]') ||
			trimmed.toLowerCase().startsWith('[drawback]')
		) {
			const clean = trimmed.replace(/^(\[-\]|-\s+|❌|\[con[s]?\]|\[drawback\])\s*/i, '');
			return { kind: 'con', clean };
		}
		if (trimmed.startsWith('[!]') || trimmed.startsWith('⚠️') || trimmed.toLowerCase().startsWith('[warn]')) {
			const clean = trimmed.replace(/^(\[!\]|⚠️|\[warn\])\s*/i, '');
			return { kind: 'warn', clean };
		}
		if (trimmed.startsWith('[?]') || trimmed.startsWith('ℹ️') || trimmed.toLowerCase().startsWith('[info]')) {
			const clean = trimmed.replace(/^(\[\?\]|ℹ️|\[info\])\s*/i, '');
			return { kind: 'info', clean };
		}
		return { kind: 'normal', clean: text };
	}

	function parseDiff(text: string) {
		return text.split('\n').map((line) => {
			if (line.startsWith('+') && !line.startsWith('+++')) return { type: 'add', line };
			if (line.startsWith('-') && !line.startsWith('---')) return { type: 'del', line };
			if (line.startsWith('@@')) return { type: 'hunk', line };
			return { type: 'ctx', line };
		});
	}

	function parseCompare(raw: string) {
		const sections = raw.split(/^###\s+/m).filter((s) => s.trim().length > 0);
		if (sections.length < 2) {
			const blocks = raw.split(/---+/).filter((s) => s.trim().length > 0);
			if (blocks.length >= 2) {
				return blocks.map((b) => {
					const lines = b.trim().split('\n');
					const title = lines[0].replace(/^#+\s*/, '').trim();
					const items = lines.slice(1).map((l) => l.trim()).filter((l) => l.length > 0);
					const isPro = /advantage|pro|positive|plus|benefit/i.test(title);
					const isCon = /drawback|con|negative|minus|risk|issue/i.test(title);
					return {
						title: title || (isPro ? 'Advantages' : 'Drawbacks'),
						tone: isPro ? 'pro' : isCon ? 'con' : 'neutral',
						items
					};
				});
			}
		}
		return sections.map((sec) => {
			const lines = sec.trim().split('\n');
			const title = lines[0].trim();
			const items = lines.slice(1).map((l) => l.trim()).filter((l) => l.length > 0);
			const isPro = /advantage|pro|positive|plus|benefit/i.test(title);
			const isCon = /drawback|con|negative|minus|risk|issue/i.test(title);
			return {
				title,
				tone: isPro ? 'pro' : isCon ? 'con' : 'neutral',
				items
			};
		});
	}

	function richInline(rawText: string): string {
		let processed = rawText;

		processed = processed.replace(
			/\[(badge|tag|chip):(success|danger|warning|info|primary|neutral)\s+([^\]]+)\]/gi,
			(_, _type, tone, label) => {
				const toneColors: Record<string, string> = {
					success: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
					danger: 'bg-rose-500/15 text-rose-300 border-rose-500/30',
					warning: 'bg-amber-500/15 text-amber-300 border-amber-500/30',
					info: 'bg-sky-500/15 text-sky-300 border-sky-500/30',
					primary: 'bg-fc-primary/15 text-fc-primary border-fc-primary/30',
					neutral: 'bg-fc-surface text-fc-fg-muted border-fc-border'
				};
				const cls = toneColors[tone.toLowerCase()] || toneColors.neutral;
				return `<span class="inline-flex items-center rounded-full border px-2 py-0.5 text-[0.7rem] font-semibold tracking-wide ${cls}">${label}</span>`;
			}
		);

		processed = processed.replace(
			/\[\+\s+([^\]]+)\]/g,
			`<span class="inline-flex items-center gap-1 rounded-full border border-emerald-500/30 bg-emerald-500/15 px-2 py-0.5 text-[0.7rem] font-semibold text-emerald-300"><span class="font-bold text-emerald-400">+</span> $1</span>`
		);

		processed = processed.replace(
			/\[-\s+([^\]]+)\]/g,
			`<span class="inline-flex items-center gap-1 rounded-full border border-rose-500/30 bg-rose-500/15 px-2 py-0.5 text-[0.7rem] font-semibold text-rose-300"><span class="font-bold text-rose-400">-</span> $1</span>`
		);

		processed = processed.replace(
			/\[status:(active|done|ready|success)\]/gi,
			`<span class="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-[0.68rem] font-semibold text-emerald-300"><span class="size-1.5 rounded-full bg-emerald-400 animate-pulse"></span>$1</span>`
		);
		processed = processed.replace(
			/\[status:(pending|wip|running|loading)\]/gi,
			`<span class="inline-flex items-center gap-1.5 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[0.68rem] font-semibold text-amber-300"><span class="size-1.5 rounded-full bg-amber-400"></span>$1</span>`
		);
		processed = processed.replace(
			/\[status:(error|failed|stopped|dead)\]/gi,
			`<span class="inline-flex items-center gap-1.5 rounded-full border border-rose-500/30 bg-rose-500/10 px-2 py-0.5 text-[0.68rem] font-semibold text-rose-300"><span class="size-1.5 rounded-full bg-rose-400"></span>$1</span>`
		);

		processed = processed.replace(
			/==([^=]+)==/g,
			`<mark class="rounded bg-amber-400/20 px-1 py-0.5 font-medium text-amber-300">$1</mark>`
		);

		return marked.parseInline(processed) as string;
	}

	function richBlock(rawText: string): string {
		let processed = rawText;

		processed = processed.replace(
			/\[(badge|tag|chip):(success|danger|warning|info|primary|neutral)\s+([^\]]+)\]/gi,
			(_, _type, tone, label) => {
				const toneColors: Record<string, string> = {
					success: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
					danger: 'bg-rose-500/15 text-rose-300 border-rose-500/30',
					warning: 'bg-amber-500/15 text-amber-300 border-amber-500/30',
					info: 'bg-sky-500/15 text-sky-300 border-sky-500/30',
					primary: 'bg-fc-primary/15 text-fc-primary border-fc-primary/30',
					neutral: 'bg-fc-surface text-fc-fg-muted border-fc-border'
				};
				const cls = toneColors[tone.toLowerCase()] || toneColors.neutral;
				return `<span class="inline-flex items-center rounded-full border px-2 py-0.5 text-[0.7rem] font-semibold tracking-wide ${cls}">${label}</span>`;
			}
		);

		processed = processed.replace(
			/\[\+\s+([^\]]+)\]/g,
			`<span class="inline-flex items-center gap-1 rounded-full border border-emerald-500/30 bg-emerald-500/15 px-2 py-0.5 text-[0.7rem] font-semibold text-emerald-300"><span class="font-bold text-emerald-400">+</span> $1</span>`
		);

		processed = processed.replace(
			/\[-\s+([^\]]+)\]/g,
			`<span class="inline-flex items-center gap-1 rounded-full border border-rose-500/30 bg-rose-500/15 px-2 py-0.5 text-[0.7rem] font-semibold text-rose-300"><span class="font-bold text-rose-400">-</span> $1</span>`
		);

		processed = processed.replace(
			/\[status:(active|done|ready|success)\]/gi,
			`<span class="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-[0.68rem] font-semibold text-emerald-300"><span class="size-1.5 rounded-full bg-emerald-400 animate-pulse"></span>$1</span>`
		);
		processed = processed.replace(
			/\[status:(pending|wip|running|loading)\]/gi,
			`<span class="inline-flex items-center gap-1.5 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[0.68rem] font-semibold text-amber-300"><span class="size-1.5 rounded-full bg-amber-400"></span>$1</span>`
		);
		processed = processed.replace(
			/\[status:(error|failed|stopped|dead)\]/gi,
			`<span class="inline-flex items-center gap-1.5 rounded-full border border-rose-500/30 bg-rose-500/10 px-2 py-0.5 text-[0.68rem] font-semibold text-rose-300"><span class="size-1.5 rounded-full bg-rose-400"></span>$1</span>`
		);

		processed = processed.replace(
			/==([^=]+)==/g,
			`<mark class="rounded bg-amber-400/20 px-1 py-0.5 font-medium text-amber-300">$1</mark>`
		);

		return marked.parse(processed) as string;
	}

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text);
		toast.success('Code copied to clipboard');
	}
</script>

<div class="flex flex-col gap-6 text-fc-fg leading-relaxed">
	{#each tokens as token}
		{#if token.type === 'heading'}
			{@const h = token as Tokens.Heading}
			{#if h.depth === 1}
				<h1 class="text-fc-2xl font-bold tracking-tight text-fc-fg sm:text-fc-3xl">
					{@html richInline(h.text)}
				</h1>
			{:else if h.depth === 2}
				<h2 class="mt-4 border-b border-fc-border pb-2 text-fc-xl font-bold tracking-tight text-fc-fg">
					{@html richInline(h.text)}
				</h2>
			{:else if h.depth === 3}
				<h3 class="mt-2 text-fc-lg font-semibold text-fc-fg">
					{@html richInline(h.text)}
				</h3>
			{:else}
				<h4 class="text-fc-base font-semibold text-fc-fg">
					{@html richInline(h.text)}
				</h4>
			{/if}

		{:else if token.type === 'paragraph'}
			{@const p = token as Tokens.Paragraph}
			<p class="text-fc-sm text-fc-fg-muted leading-relaxed">
				{@html richInline(p.text)}
			</p>

		{:else if token.type === 'blockquote'}
			{@const b = token as Tokens.Blockquote}
			{@const alert = parseAlert(b.text)}
			{#if alert.isAlert}
				{#if alert.tone === 'pros' || alert.tone === 'tip'}
					<div class="my-3 flex flex-col gap-1.5 rounded-fc-lg border-l-4 border-emerald-500 bg-emerald-500/10 p-4 text-fc-sm shadow-fc-xs">
						<div class="flex items-center gap-2 font-semibold text-emerald-400">
							<span class="inline-flex size-5 items-center justify-center rounded-full bg-emerald-500/20 text-xs font-bold">+</span>
							<span>{alert.title}</span>
						</div>
						<div class="text-fc-xs text-emerald-200/90 leading-relaxed pl-7 flex flex-col gap-1.5 [&>h1]:text-fc-base [&>h1]:font-bold [&>h2]:text-fc-sm [&>h2]:font-bold [&>h3]:text-fc-sm [&>h3]:font-semibold [&>h3]:text-emerald-300 [&>ul]:list-disc [&>ul]:pl-5 [&>ul]:space-y-1 [&>ol]:list-decimal [&>ol]:pl-5 [&>ol]:space-y-1">
							{@html richBlock(alert.body)}
						</div>
					</div>
				{:else if alert.tone === 'cons' || alert.tone === 'caution'}
					<div class="my-3 flex flex-col gap-1.5 rounded-fc-lg border-l-4 border-rose-500 bg-rose-500/10 p-4 text-fc-sm shadow-fc-xs">
						<div class="flex items-center gap-2 font-semibold text-rose-400">
							<span class="inline-flex size-5 items-center justify-center rounded-full bg-rose-500/20 text-xs font-bold">-</span>
							<span>{alert.title}</span>
						</div>
						<div class="text-fc-xs text-rose-200/90 leading-relaxed pl-7 flex flex-col gap-1.5 [&>h1]:text-fc-base [&>h1]:font-bold [&>h2]:text-fc-sm [&>h2]:font-bold [&>h3]:text-fc-sm [&>h3]:font-semibold [&>h3]:text-rose-300 [&>ul]:list-disc [&>ul]:pl-5 [&>ul]:space-y-1 [&>ol]:list-decimal [&>ol]:pl-5 [&>ol]:space-y-1">
							{@html richBlock(alert.body)}
						</div>
					</div>
				{:else if alert.tone === 'warning'}
					<div class="my-3 flex flex-col gap-1.5 rounded-fc-lg border-l-4 border-amber-500 bg-amber-500/10 p-4 text-fc-sm shadow-fc-xs">
						<div class="flex items-center gap-2 font-semibold text-amber-400">
							<span class="inline-flex size-5 items-center justify-center rounded-full bg-amber-500/20 text-xs font-bold">!</span>
							<span>{alert.title}</span>
						</div>
						<div class="text-fc-xs text-amber-200/90 leading-relaxed pl-7 flex flex-col gap-1.5 [&>h1]:text-fc-base [&>h1]:font-bold [&>h2]:text-fc-sm [&>h2]:font-bold [&>h3]:text-fc-sm [&>h3]:font-semibold [&>h3]:text-amber-300 [&>ul]:list-disc [&>ul]:pl-5 [&>ul]:space-y-1 [&>ol]:list-decimal [&>ol]:pl-5 [&>ol]:space-y-1">
							{@html richBlock(alert.body)}
						</div>
					</div>
				{:else if alert.tone === 'important'}
					<div class="my-3 flex flex-col gap-1.5 rounded-fc-lg border-l-4 border-purple-500 bg-purple-500/10 p-4 text-fc-sm shadow-fc-xs">
						<div class="flex items-center gap-2 font-semibold text-purple-400">
							<span class="inline-flex size-5 items-center justify-center rounded-full bg-purple-500/20 text-xs font-bold">★</span>
							<span>{alert.title}</span>
						</div>
						<div class="text-fc-xs text-purple-200/90 leading-relaxed pl-7 flex flex-col gap-1.5 [&>h1]:text-fc-base [&>h1]:font-bold [&>h2]:text-fc-sm [&>h2]:font-bold [&>h3]:text-fc-sm [&>h3]:font-semibold [&>h3]:text-purple-300 [&>ul]:list-disc [&>ul]:pl-5 [&>ul]:space-y-1 [&>ol]:list-decimal [&>ol]:pl-5 [&>ol]:space-y-1">
							{@html richBlock(alert.body)}
						</div>
					</div>
				{:else}
					<div class="my-3 flex flex-col gap-1.5 rounded-fc-lg border-l-4 border-sky-500 bg-sky-500/10 p-4 text-fc-sm shadow-fc-xs">
						<div class="flex items-center gap-2 font-semibold text-sky-400">
							<span class="inline-flex size-5 items-center justify-center rounded-full bg-sky-500/20 text-xs font-bold">i</span>
							<span>{alert.title}</span>
						</div>
						<div class="text-fc-xs text-sky-200/90 leading-relaxed pl-7 flex flex-col gap-1.5 [&>h1]:text-fc-base [&>h1]:font-bold [&>h2]:text-fc-sm [&>h2]:font-bold [&>h3]:text-fc-sm [&>h3]:font-semibold [&>h3]:text-sky-300 [&>ul]:list-disc [&>ul]:pl-5 [&>ul]:space-y-1 [&>ol]:list-decimal [&>ol]:pl-5 [&>ol]:space-y-1">
							{@html richBlock(alert.body)}
						</div>
					</div>
				{/if}
			{:else}
				<div class="rounded-fc-lg border-l-4 border-fc-primary bg-fc-surface/60 p-4 text-fc-sm text-fc-fg-muted italic">
					{@html marked.parse(b.text)}
				</div>
			{/if}

		{:else if token.type === 'list'}
			{@const l = token as Tokens.List}
			{#if l.ordered}
				<ol class="flex flex-col gap-2 pl-2" start={typeof l.start === 'number' ? l.start : 1}>
					{#each l.items as item, index}
						{@const parsed = parseListItem(item.text)}
						<li class="flex items-start gap-3 text-fc-sm text-fc-fg">
							<span class="flex size-5 shrink-0 items-center justify-center rounded-full bg-fc-surface border border-fc-border font-fc-mono text-[0.7rem] font-semibold text-fc-primary">
								{(typeof l.start === 'number' ? l.start : 1) + index}
							</span>
							<div class="leading-relaxed text-fc-fg-muted pt-0.5">
								{@html richInline(parsed.clean)}
							</div>
						</li>
					{/each}
				</ol>
			{:else}
				<ul class="flex flex-col gap-2">
					{#each l.items as item}
						{@const parsed = parseListItem(item.text)}
						{#if parsed.kind === 'pro'}
							<li class="flex items-start gap-2.5 rounded-fc-md bg-emerald-500/5 px-3 py-1.5 text-fc-sm border border-emerald-500/20">
								<span class="mt-0.5 inline-flex size-4 shrink-0 items-center justify-center rounded-full bg-emerald-500/20 text-emerald-400 text-xs font-bold">
									+
								</span>
								<div class="leading-relaxed text-emerald-200/90">
									{@html richInline(parsed.clean)}
								</div>
							</li>
						{:else if parsed.kind === 'con'}
							<li class="flex items-start gap-2.5 rounded-fc-md bg-rose-500/5 px-3 py-1.5 text-fc-sm border border-rose-500/20">
								<span class="mt-0.5 inline-flex size-4 shrink-0 items-center justify-center rounded-full bg-rose-500/20 text-rose-400 text-xs font-bold">
									-
								</span>
								<div class="leading-relaxed text-rose-200/90">
									{@html richInline(parsed.clean)}
								</div>
							</li>
						{:else if parsed.kind === 'warn'}
							<li class="flex items-start gap-2.5 rounded-fc-md bg-amber-500/5 px-3 py-1.5 text-fc-sm border border-amber-500/20">
								<span class="mt-0.5 inline-flex size-4 shrink-0 items-center justify-center rounded-full bg-amber-500/20 text-amber-400 text-xs font-bold">
									!
								</span>
								<div class="leading-relaxed text-amber-200/90">
									{@html richInline(parsed.clean)}
								</div>
							</li>
						{:else if parsed.kind === 'info'}
							<li class="flex items-start gap-2.5 rounded-fc-md bg-sky-500/5 px-3 py-1.5 text-fc-sm border border-sky-500/20">
								<span class="mt-0.5 inline-flex size-4 shrink-0 items-center justify-center rounded-full bg-sky-500/20 text-sky-400 text-xs font-bold">
									i
								</span>
								<div class="leading-relaxed text-sky-200/90">
									{@html richInline(parsed.clean)}
								</div>
							</li>
						{:else}
							<li class="flex items-start gap-2.5 pl-2 text-fc-sm text-fc-fg-muted">
								<span class="mt-2 size-1.5 shrink-0 rounded-full bg-fc-primary/80"></span>
								<div class="leading-relaxed">
									{@html richInline(parsed.clean)}
								</div>
							</li>
						{/if}
					{/each}
				</ul>
			{/if}

		{:else if token.type === 'table'}
			{@const t = token as Tokens.Table}
			<div class="my-3 overflow-x-auto rounded-fc-lg border border-fc-border bg-fc-surface/40">
				<table class="w-full border-collapse text-fc-sm">
					<thead>
						<tr class="border-b border-fc-border bg-fc-surface-hover/60">
							{#each t.header as headerCell, i}
								<th class="p-3 text-left font-semibold text-fc-fg {t.align[i] ? `text-${t.align[i]}` : ''}">
									{@html richInline(headerCell.text)}
								</th>
							{/each}
						</tr>
					</thead>
					<tbody class="divide-y divide-fc-border/60">
						{#each t.rows as row}
							<tr class="transition-colors hover:bg-fc-surface-hover/40">
								{#each row as cell, i}
									<td class="p-3 text-fc-fg-muted {t.align[i] ? `text-${t.align[i]}` : ''}">
										{@html richInline(cell.text)}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

		{:else if token.type === 'code'}
			{@const c = token as Tokens.Code}
			{@const lang = (c.lang || '').toLowerCase().trim()}

			{#if lang.startsWith('chart:bar') || lang.startsWith('chart:bars') || lang === 'chart'}
				{@const chart = parseBarChart(c.text)}
				<div class="my-4 rounded-fc-lg border border-fc-border bg-fc-surface p-5 shadow-fc-xs">
					{#if chart.title}
						<h4 class="mb-4 text-fc-sm font-semibold text-fc-fg">{chart.title}</h4>
					{/if}
					<div class="flex flex-col gap-3.5">
						{#each chart.items as item}
							{@const pct = Math.min(Math.round((item.value / chart.max) * 100), 100)}
							<div class="flex flex-col gap-1 text-fc-xs">
								<div class="flex justify-between font-medium text-fc-fg">
									<span>{item.label}</span>
									<span class="font-fc-mono text-fc-fg-muted">{item.formatted}</span>
								</div>
								<div class="h-2 w-full overflow-hidden rounded-full bg-fc-surface-hover">
									<div class="h-full rounded-full bg-fc-primary transition-all duration-500" style="width: {pct}%"></div>
								</div>
							</div>
						{/each}
					</div>
				</div>

			{:else if lang.startsWith('chart:metric') || lang.startsWith('chart:metrics')}
				{@const metrics = parseMetrics(c.text)}
				<div class="my-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
					{#each metrics as m}
						<div class="flex flex-col gap-1 rounded-fc-lg border border-fc-border bg-fc-surface p-4 shadow-fc-xs">
							{#if m.label}
								<span class="text-[0.7rem] font-semibold uppercase tracking-wider text-fc-fg-muted">{m.label}</span>
							{/if}
							{#if m.val}
								<span class="text-fc-xl font-bold tracking-tight text-fc-fg">{m.val}</span>
							{/if}
							{#if m.sub}
								<span class="text-fc-xs text-fc-fg-muted">{m.sub}</span>
							{/if}
						</div>
					{/each}
				</div>

			{:else if lang === 'compare' || lang === 'pros-cons' || lang === 'vs'}
				{@const comp = parseCompare(c.text)}
				<div class="my-4 grid gap-4 sm:grid-cols-2">
					{#each comp as section}
						<div class="flex flex-col gap-3 rounded-fc-lg border p-4 {section.tone === 'pro' ? 'border-emerald-500/30 bg-emerald-500/5' : section.tone === 'con' ? 'border-rose-500/30 bg-rose-500/5' : 'border-fc-border bg-fc-surface'}">
							<div class="flex items-center gap-2 font-semibold {section.tone === 'pro' ? 'text-emerald-400' : section.tone === 'con' ? 'text-rose-400' : 'text-fc-fg'}">
								<span class="inline-flex size-5 items-center justify-center rounded-full {section.tone === 'pro' ? 'bg-emerald-500/20 text-xs font-bold' : section.tone === 'con' ? 'bg-rose-500/20 text-xs font-bold' : 'bg-fc-surface-hover'}">
									{section.tone === 'pro' ? '+' : section.tone === 'con' ? '-' : '•'}
								</span>
								<span>{section.title}</span>
							</div>
							<ul class="flex flex-col gap-2 text-fc-xs text-fc-fg-muted">
								{#each section.items as item}
									{@const parsed = parseListItem(item)}
									<li class="flex items-start gap-2">
										<span class="mt-1 size-1 shrink-0 rounded-full {section.tone === 'pro' ? 'bg-emerald-400' : section.tone === 'con' ? 'bg-rose-400' : 'bg-fc-primary'}"></span>
										<span class="leading-relaxed">{@html richInline(parsed.clean)}</span>
									</li>
								{/each}
							</ul>
						</div>
					{/each}
				</div>

			{:else if lang === 'diff'}
				{@const diffLines = parseDiff(c.text)}
				<div class="my-3 overflow-hidden rounded-fc-lg border border-fc-border bg-fc-surface font-fc-mono text-fc-xs">
					<div class="flex items-center justify-between border-b border-fc-border bg-fc-surface-hover/40 px-4 py-1.5 text-[0.7rem] font-semibold uppercase tracking-wider text-fc-fg-muted">
						<span>diff</span>
						<Button variant="ghost" size="sm" icon={icons.copy} onclick={() => copyToClipboard(c.text)}>
							Copy
						</Button>
					</div>
					<pre class="overflow-x-auto p-3 leading-relaxed"><code>{#each diffLines as dl}<div class="{dl.type === 'add' ? 'bg-emerald-500/15 text-emerald-300 font-semibold' : dl.type === 'del' ? 'bg-rose-500/15 text-rose-300 font-semibold' : dl.type === 'hunk' ? 'bg-sky-500/15 text-sky-300 italic' : 'text-fc-fg-muted'} px-2 py-0.5">{dl.line}</div>{/each}</code></pre>
				</div>

			{:else if lang === 'mermaid'}
				<MermaidBlock code={c.text} />

			{:else}
				<div class="my-3 overflow-hidden rounded-fc-lg border border-fc-border bg-fc-surface">
					{#if c.lang}
						<div class="flex items-center justify-between border-b border-fc-border bg-fc-surface-hover/40 px-4 py-1.5 text-[0.7rem] font-semibold uppercase tracking-wider text-fc-fg-muted">
							<span>{c.lang}</span>
							<Button variant="ghost" size="sm" icon={icons.copy} onclick={() => copyToClipboard(c.text)}>
								Copy
							</Button>
						</div>
					{/if}
					<pre class="overflow-x-auto p-4 font-fc-mono text-fc-xs leading-relaxed text-fc-fg"><code>{c.text}</code></pre>
				</div>
			{/if}

		{:else if token.type === 'hr'}
			<hr class="my-4 border-fc-border" />
		{/if}
	{/each}
</div>
