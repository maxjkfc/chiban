import { cn } from "@/lib/utils";

/** The four decorative tape colours. They carry no meaning — see DESIGN_SYSTEM.md. */
const tones = {
  1: "bg-tape-1/30 border-tape-1/55",
  2: "bg-tape-2/30 border-tape-2/55",
  3: "bg-tape-3/30 border-tape-3/55",
  4: "bg-tape-4/30 border-tape-4/55",
} as const;

type TapeProps = {
  tone?: keyof typeof tones;
  /** Position and rotation. The parent must be `relative`. */
  className?: string;
};

/**
 * A strip of washi tape stuck over a card's top edge.
 *
 * Decoration, so it is hidden from assistive technology and takes no pointer
 * events: it overlaps the card it decorates, and a tape that swallowed taps
 * would put a dead strip across the top of every meal.
 */
export function Tape({ tone = 2, className }: TapeProps) {
  return (
    <span aria-hidden className={cn("tape", tones[tone], className)} />
  );
}

/**
 * Which way a card leans, and by how much.
 *
 * Derived from the item's position rather than randomised, so a card keeps its
 * angle across re-renders — a list that reshuffled its tilt whenever a message
 * arrived would look broken rather than hand-made. Kept inside ±1.5°: past
 * that the text starts to read as crooked instead of casual.
 */
const tilts = ["-rotate-[1.4deg]", "rotate-[1.1deg]", "-rotate-[0.8deg]", "rotate-[1.5deg]"] as const;

export function tiltClass(index: number): string {
  return tilts[index % tilts.length];
}

/** The tape colour that goes with a card at this position. */
export function tapeTone(index: number): keyof typeof tones {
  return ((index % 4) + 1) as keyof typeof tones;
}

/** Where the tape sits, alternating so a list does not look stencilled. */
const placements = [
  "-top-2.5 left-6 w-[74px] -rotate-[4deg]",
  "-top-2.5 right-8 w-[74px] rotate-[5deg]",
  "-top-2.5 left-[40%] w-[74px] -rotate-[2deg]",
  "-top-2.5 right-[30%] w-[74px] rotate-[3deg]",
] as const;

export function tapePlacement(index: number): string {
  return placements[index % placements.length];
}
