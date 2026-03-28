<script lang="ts">
	import type { SurfaceMeta } from '$lib/config/surfaces';

	let { surface }: { surface: SurfaceMeta } = $props();

	function toneClass(tone: 'default' | 'accent' | 'warning' | undefined): string {
		if (tone === 'accent') {
			return 'gc-card-accent';
		}
		if (tone === 'warning') {
			return 'gc-card-warning';
		}
		return '';
	}
</script>

<div class="grid gap-6 xl:grid-cols-[minmax(0,1.4fr)_minmax(18rem,0.8fr)]">
	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<p class="gc-stamp">{surface.workspaceEyebrow}</p>
		<h2 class="gc-section-title mt-3">{surface.workspaceTitle}</h2>
		<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">{surface.workspaceBody}</p>

		<div class="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
			{#each surface.cards as card (card.label)}
				<article class={`gc-panel-soft px-4 py-4 ${toneClass(card.tone)}`}>
					<p class="gc-stamp">{card.label}</p>
					<p class="gc-value mt-3">{card.value}</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{card.detail}</p>
				</article>
			{/each}
		</div>
	</section>

	<section class="grid gap-4">
		<div class="gc-panel px-4 py-4">
			<p class="gc-stamp">Operator stance</p>
			<h3 class="gc-panel-title mt-3">Design from the user’s job first</h3>
			<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
				Each surface explains what the operator can do now, what the assistant is doing, and what
				needs a decision next.
			</p>
		</div>

		<div class="gc-panel-soft px-4 py-4">
			<p class="gc-stamp">Rewrite note</p>
			<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
				This placeholder surface is the new shell baseline. The next iterations will replace these
				cards with live Go-backed data and XYFlow modules.
			</p>
		</div>
	</section>
</div>
