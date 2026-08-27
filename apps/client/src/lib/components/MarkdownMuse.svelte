<script lang="ts">
	import { Badge, Button, Card, icons, toast } from '@facile/muse';
	import { marked, type Tokens, type Token } from 'marked';

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

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text);
		toast.success('Code copied to clipboard');
	}
</script>

<div class="flex flex-col gap-5 text-fc-fg leading-relaxed">
	{#each tokens as token}
		{#if token.type === 'heading'}
			{@const h = token as Tokens.Heading}
			{#if h.depth === 1}
				<h1 class="text-fc-2xl font-bold tracking-tight text-fc-fg sm:text-fc-3xl">
					{@html marked.parseInline(h.text)}
				</h1>
			{:else if h.depth === 2}
				<h2 class="mt-4 border-b border-fc-border pb-2 text-fc-xl font-bold tracking-tight text-fc-fg">
					{@html marked.parseInline(h.text)}
				</h2>
			{:else if h.depth === 3}
				<h3 class="mt-2 text-fc-lg font-semibold text-fc-fg">
					{@html marked.parseInline(h.text)}
				</h3>
			{:else}
				<h4 class="text-fc-base font-semibold text-fc-fg">
					{@html marked.parseInline(h.text)}
				</h4>
			{/if}

		{:else if token.type === 'paragraph'}
			{@const p = token as Tokens.Paragraph}
			<p class="text-fc-sm text-fc-fg-muted leading-relaxed">
				{@html marked.parseInline(p.text)}
			</p>

		{:else if token.type === 'blockquote'}
			{@const b = token as Tokens.Blockquote}
			<div class="rounded-fc-lg border-l-4 border-fc-primary bg-fc-surface p-4 text-fc-sm text-fc-fg-muted italic">
				{@html marked.parse(b.text)}
			</div>

		{:else if token.type === 'list'}
			{@const l = token as Tokens.List}
			{#if l.ordered}
				<ol class="list-decimal pl-5 text-fc-sm text-fc-fg-muted flex flex-col gap-1.5" start={typeof l.start === 'number' ? l.start : 1}>
					{#each l.items as item}
						<li>{@html marked.parseInline(item.text)}</li>
					{/each}
				</ol>
			{:else}
				<ul class="list-disc pl-5 text-fc-sm text-fc-fg-muted flex flex-col gap-1.5">
					{#each l.items as item}
						<li>{@html marked.parseInline(item.text)}</li>
					{/each}
				</ul>
			{/if}

		{:else if token.type === 'table'}
			{@const t = token as Tokens.Table}
			<div class="my-3 overflow-x-auto rounded-fc-lg border border-fc-border bg-fc-surface">
				<table class="w-full border-collapse text-fc-sm">
					<thead>
						<tr class="border-b border-fc-border bg-fc-surface-hover/60">
							{#each t.header as headerCell, i}
								<th class="p-3 text-left font-semibold text-fc-fg {t.align[i] ? `text-${t.align[i]}` : ''}">
									{@html marked.parseInline(headerCell.text)}
								</th>
							{/each}
						</tr>
					</thead>
					<tbody class="divide-y divide-fc-border/60">
						{#each t.rows as row}
							<tr class="transition-colors hover:bg-fc-surface-hover/40">
								{#each row as cell, i}
									<td class="p-3 text-fc-fg-muted {t.align[i] ? `text-${t.align[i]}` : ''}">
										{@html marked.parseInline(cell.text)}
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
				<Card class="my-4 p-5">
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
				</Card>

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

			{:else if lang === 'mermaid'}
				<Card class="my-4 overflow-hidden p-0">
					<div class="flex items-center justify-between border-b border-fc-border bg-fc-surface-hover/40 px-4 py-2 text-fc-xs text-fc-fg-muted">
						<span class="font-semibold uppercase tracking-wider">Mermaid Flow</span>
						<Button variant="ghost" size="sm" icon={icons.copy} onclick={() => copyToClipboard(c.text)}>
							Copy
						</Button>
					</div>
					<pre class="overflow-x-auto p-4 font-fc-mono text-fc-xs text-fc-fg"><code>{c.text}</code></pre>
				</Card>

			{:else}
				<Card class="my-3 overflow-hidden p-0">
					{#if c.lang}
						<div class="flex items-center justify-between border-b border-fc-border bg-fc-surface-hover/40 px-4 py-1.5 text-[0.7rem] font-semibold uppercase tracking-wider text-fc-fg-muted">
							<span>{c.lang}</span>
							<Button variant="ghost" size="sm" icon={icons.copy} onclick={() => copyToClipboard(c.text)}>
								Copy
							</Button>
						</div>
					{/if}
					<pre class="overflow-x-auto p-4 font-fc-mono text-fc-xs leading-relaxed text-fc-fg"><code>{c.text}</code></pre>
				</Card>
			{/if}

		{:else if token.type === 'hr'}
			<hr class="my-4 border-fc-border" />
		{/if}
	{/each}
</div>
