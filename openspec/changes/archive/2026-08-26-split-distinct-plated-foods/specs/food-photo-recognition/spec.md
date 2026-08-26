## MODIFIED Requirements

### Requirement: Composite Dish Naming

Recognize SHALL return one item per independently served, portion-scale food visible in the
photo, and SHALL merge components into a single item when they were mixed, chopped, tossed,
cooked, or sauced together into one combined served preparation (e.g. a curry, a stew, a
stir-fry, a pre-mixed side, a mixed salad) — even when individual ingredient pieces within that
preparation remain visually distinguishable on close inspection (e.g. a piece of lettuce or a
cucumber slice within a chopped salad, or a chunk of carrot within a stir-fry); those pieces are
components of one recipe, not separately-served foods, and do not each become their own item.
Being cooked, baked, or braised in physical contact with another food — sharing a pan, a pot, or
an oven dish — is not by itself what makes something a combined preparation: a whole,
portion-scale piece that keeps its own shape and could be lifted off and served on its own, such
as a fish fillet, a chicken breast, or a cutlet, remains a separately-served food even when it was
cooked in contact with another food (e.g. a fish fillet baked on top of a bed of stewed cabbage).
The "cooked or sauced together" merge trigger applies only when the cooking process itself blends
the components so that no piece remains in a separately-servable, portion-scale form — not merely
when two whole foods happen to share a cooking vessel.
Two or more foods that are each independently served as their own portion-scale food — for
example a protein and a vegetable or starch side, but equally two foods that happen to share a
role, such as two different cooked vegetables plated touching — SHALL be returned as separate
items even when they are plated touching each other, one resting on or against the other, or
otherwise not spatially separated into distinct piles. The distinction is not whether a piece
can be pointed to and named (an ingredient chunk within a combined preparation almost always
can), but whether it was ever served as its own separate portion rather than combined into one
dish — and, since Recognize sees only the photo, this is judged from what the photo actually
shows: a piece that visibly keeps its own separately-servable, portion-scale form (an intact
fillet, a whole cutlet, a distinct pile) counts as served separately; a piece broken down, mixed,
or tossed into one preparation does not, regardless of how it was actually plated before
cooking. A sauce, glaze, or juices from a neighboring food covering a piece does not by itself
turn it into a combined preparation — a fillet still counts as its own separately-servable piece
even when it is coated in sauce, as long as it keeps its own portion-scale shape.
A minor garnish, condiment, or drizzle (e.g. a lemon wedge, a sprig of herbs, a spoonful of
sauce) that accompanies a main component rather than constituting its own portion-sized food
SHALL stay folded into the item it accompanies, even though it is technically independently
namable. This granularity judgment is made by the model on each photo; it is not a user-facing
setting and there is no per-upload or per-user toggle for it.

#### Scenario: A single composite dish is recognized as one item

- **WHEN** a photo shows a homogeneous composite dish, such as a mixed vegetable side, a
  stir-fry, or a sauced dish, where the ingredients were mixed, chopped, or cooked together into
  one served preparation — even if individual ingredient pieces (e.g. a piece of lettuce or a
  chunk of carrot) remain visually distinguishable on close inspection
- **THEN** Recognize returns exactly one item for that dish, named to reflect the dish as a
  whole, rather than one item per ingredient it may contain

#### Scenario: A plate with visually separate components is still decomposed

- **WHEN** a photo shows a plate with clearly separate components, such as a portion of rice, a
  piece of grilled protein, and a side salad plated apart from each other
- **THEN** Recognize returns one item per visually separate component, as it does today

#### Scenario: Separately-served foods plated touching each other are still split

- **WHEN** a photo shows two foods that were each served as their own separate portion but are
  plated touching or resting against each other rather than plated apart — such as a piece of
  white fish served on or next to a portion of stewed cabbage — whether or not the two foods
  share the same role (e.g. two different cooked vegetable sides touching are split just as a
  protein-and-side pairing is)
- **THEN** Recognize returns one item for each of the separately-served foods (e.g. one item for
  the fish, one item for the stewed cabbage), rather than merging them into a single named dish

#### Scenario: A portion-scale piece cooked in contact with another food is still split

- **WHEN** a photo shows a whole, portion-scale piece of one food — such as a fish fillet, a
  chicken breast, or a cutlet — that was baked, braised, or otherwise cooked in physical contact
  with another food (e.g. a fish fillet baked directly on top of a bed of stewed cabbage, so the
  two were in the same dish for the whole cooking process), but the piece still keeps its own
  shape and could be lifted off and served on its own
- **THEN** Recognize still returns one item for each food (e.g. one item for the fish, one item
  for the cabbage) rather than merging them, because sharing a cooking vessel or process is not
  by itself what makes a combined preparation — only losing separately-servable, portion-scale
  form does

#### Scenario: A minor garnish or condiment stays folded into its main component

- **WHEN** a photo shows a main food accompanied by a minor garnish, condiment, or drizzle — such
  as a lemon wedge, a sprig of herbs, or a spoonful of sauce — that is not itself a portion-sized
  food
- **THEN** Recognize returns one item for the main food and does not return a separate item for
  the garnish or condiment
