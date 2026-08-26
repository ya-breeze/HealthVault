import { ComponentPropsWithRef, ElementType } from 'react';

// Minimum interactive-control size for the food flow: 48x48 CSS px, the
// Android Material Design guideline (see openspec/changes/mobile-tap-targets
// /design.md — chosen over the 44px iOS/WCAG figure to match the actual test
// device). Use TapTarget for any tappable control here instead of a bare
// <button>/<Link>, so meeting the minimum doesn't depend on each call site
// remembering to size itself: `min-h-12 min-w-12` is 12 * 0.25rem = 48px on
// Tailwind's default (unmodified) spacing scale.
const MIN_TAP_TARGET_CLASSES = 'min-h-12 min-w-12';

// Same minimum, released only where the primary pointing device is a mouse.
// For compact segmented controls whose pointer-precise rendering is
// deliberately small, and where a 48px control would just look chunky.
//
// Keyed off pointer type, NOT viewport width. A width test is the obvious
// implementation and it is wrong: a 375x667 phone in landscape is 667 CSS px
// wide, so any `sm:`-style release (>=640px) hands the fix back on exactly the
// device this change was filed for, as does a tablet in portrait at 768px.
// `pointer: fine` asks the question actually being asked — "is this a mouse?"
// — and it fails safe, since a device that reports no fine pointer keeps the
// minimum.
//
// Kept here rather than as opt-out classes at the call site, so sizing still
// lives in exactly one place; a call site asks for the behaviour by name and
// never spells out its own dimensions.
//
// Releases to `auto`, not `0`: `auto` is what these properties computed to
// before this component was applied, so the release restores the original
// rendering exactly. `min-width: 0` would go further and also switch off a
// flex item's automatic minimum size — both call sites are flex children, and
// that would let a cramped row shrink a label below its own content width
// instead of leaving it intact.
const COMPACT_ON_MOUSE_CLASSES = `${MIN_TAP_TARGET_CLASSES} pointer-fine:min-h-auto pointer-fine:min-w-auto`;

type TapTargetProps<T extends ElementType> = {
  as?: T;
  /**
   * Render at the control's natural size where the primary pointing device is
   * fine (a mouse); take the 48px minimum everywhere else, including on a
   * device that reports no pointer at all.
   */
  compactOnMouse?: boolean;
  // `ComponentPropsWithRef`, not `…WithoutRef`: BottomNav's More destination
  // has to be re-focused when the sheet it opens closes, so a caller must be
  // able to hold a ref to the rendered element. React 19 passes `ref`
  // through as an ordinary prop for function components, so the spread below
  // delivers it without a forwardRef wrapper — only the type had to widen.
} & ComponentPropsWithRef<T>;

// Spreads all other props through unchanged (title, aria-label, data-testid,
// disabled, onClick, href, ref, ...) — several existing controls are located
// by these attributes in e2e/tests/food.spec.ts, and migrating a control to
// TapTarget must not change how it's identified by other code.
export default function TapTarget<T extends ElementType = 'button'>({
  as,
  className,
  compactOnMouse,
  ...rest
}: TapTargetProps<T>) {
  const Component = (as ?? 'button') as ElementType;
  // Destructured above so it is never spread onto the DOM element below.
  const minClasses = compactOnMouse ? COMPACT_ON_MOUSE_CLASSES : MIN_TAP_TARGET_CLASSES;
  const mergedClassName = className ? `${minClasses} ${className}` : minClasses;
  // Spread before {...rest} below, so an explicit `type` in rest overrides this default.
  const defaultType = Component === 'button' ? { type: 'button' as const } : undefined;

  return <Component className={mergedClassName} {...defaultType} {...rest} />;
}
