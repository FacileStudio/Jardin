<script lang="ts">
    import { onMount } from 'svelte';
    import { gsap } from 'gsap';
    import { ScrollTrigger } from 'gsap/ScrollTrigger';
    import { SplitText } from 'gsap/SplitText';
    import { twMerge, prefersReducedMotion } from '@facile/muse';

    function scrollParent(node: Element | null): HTMLElement | undefined {
        if (typeof window === 'undefined' || !node) return undefined;
        let current = node.parentElement;
        while (current && current !== document.body && current !== document.documentElement) {
            const { overflowY } = getComputedStyle(current);
            if (overflowY === 'auto' || overflowY === 'scroll') return current;
            current = current.parentElement;
        }
        return undefined;
    }

    let {
        text,
        dimColor = 'color-mix(in oklab, var(--color-fc-fg) 25%, transparent)',
        revealColor = 'var(--color-fc-fg)',
        scroller,
        class: className = ''
    }: {
        text: string;
        dimColor?: string;
        revealColor?: string;
        scroller?: HTMLElement | null;
        class?: string;
    } = $props();

    let el: HTMLParagraphElement | null = $state(null);

    const classes = $derived(twMerge('leading-loose text-fc-lg max-w-[60ch]', className));

    /*
        The ScrollTrigger created here outlives the component unless it is explicitly
        killed: it stays on ScrollTrigger's global list and recomputes on every scroll
        event, holding the detached paragraph and its word spans alive. A gsap.context
        scoped to the element collects the tween and its trigger so one revert() drops
        both; SplitText is reverted separately because it is not a gsap animation.
    */
    onMount(() => {
        const node = el;
        if (!node) return;
        if (prefersReducedMotion()) {
            node.style.color = revealColor;
            return;
        }
        gsap.registerPlugin(ScrollTrigger, SplitText);
        const split = SplitText.create(node, { type: 'words' });

        /*
            ScrollTrigger listens on the window by default, and in the suite's standard shell
            nothing ever scrolls the window — the page is inside a `<main class="overflow-auto">`
            beside a fixed rail, and scroll events do not bubble out of that container. Without
            an explicit scroller this component sits at progress 0 forever and reads as broken.
            Auto-detected, with a prop to override when the detection is wrong.
        */
        const scrollerEl = scroller ?? scrollParent(node);

        const ctx = gsap.context(() => {
            gsap.to(split.words, {
                color: revealColor,
                stagger: 0.1,
                ease: 'power3.inOut',
                scrollTrigger: {
                    trigger: node,
                    scroller: scrollerEl,
                    start: 'top center',
                    end: 'bottom center',
                    scrub: true
                }
            });
        }, node);

        return () => {
            ctx.revert();
            split.revert();
        };
    });
</script>

<p bind:this={el} class={classes} style:color={dimColor}>{text}</p>
