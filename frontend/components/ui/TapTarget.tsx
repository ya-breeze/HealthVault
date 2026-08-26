import { ComponentPropsWithoutRef, ElementType } from 'react';

// Minimum interactive-control size for the food flow: 48x48 CSS px, the
// Android Material Design guideline (see openspec/changes/mobile-tap-targets
// /design.md — chosen over the 44px iOS/WCAG figure to match the actual test
// device). Use TapTarget for any tappable control here instead of a bare
// <button>/<Link>, so meeting the minimum doesn't depend on each call site
// remembering to size itself: `min-h-12 min-w-12` is 12 * 0.25rem = 48px on
// Tailwind's default (unmodified) spacing scale.
const MIN_TAP_TARGET_CLASSES = 'min-h-12 min-w-12';

// Same minimum on touch-sized viewports, released at `sm` (>=640px). For
// compact segmented controls whose desktop rendering is deliberately small
// and where a 48px control would just look chunky — the tap-target problem
// they solve only exists on a phone. Kept here rather than as opt-out classes
// at the call site, so sizing still lives in exactly one place; a call site
// asks for the behaviour by name and never spells out its own dimensions.
const MOBILE_ONLY_CLASSES = `${MIN_TAP_TARGET_CLASSES} sm:min-h-0 sm:min-w-0`;

type TapTargetProps<T extends ElementType> = {
  as?: T;
  mobileOnly?: boolean;
} & ComponentPropsWithoutRef<T>;

// Spreads all other props through unchanged (title, aria-label, data-testid,
// disabled, onClick, href, ...) — several existing controls are located by
// these attributes in e2e/tests/food.spec.ts, and migrating a control to
// TapTarget must not change how it's identified by other code.
export default function TapTarget<T extends ElementType = 'button'>({
  as,
  className,
  mobileOnly,
  ...rest
}: TapTargetProps<T>) {
  const Component = (as ?? 'button') as ElementType;
  // Destructured above so it is never spread onto the DOM element below.
  const minClasses = mobileOnly ? MOBILE_ONLY_CLASSES : MIN_TAP_TARGET_CLASSES;
  const mergedClassName = className ? `${minClasses} ${className}` : minClasses;
  // Spread before {...rest} below, so an explicit `type` in rest overrides this default.
  const defaultType = Component === 'button' ? { type: 'button' as const } : undefined;

  return <Component className={mergedClassName} {...defaultType} {...rest} />;
}
