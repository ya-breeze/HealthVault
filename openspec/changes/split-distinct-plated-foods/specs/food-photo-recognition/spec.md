## MODIFIED Requirements

### Requirement: Composite Dish Naming

Recognize SHALL return one item per independently identifiable food visible in the photo, and
SHALL merge components into a single item only when they no longer have independent identity —
i.e. when the visible mass is one homogeneous preparation whose ingredients have been combined
and cooked/sauced together (e.g. a curry, a stew, a stir-fry, a pre-mixed side, a mixed salad)
such that no individual component can be pointed to and named on its own. Two or more foods that
each remain independently namable and visually distinguishable — for example a protein and a
vegetable or starch side, but equally two foods that happen to share a role, such as two
different cooked vegetables plated touching — SHALL be returned as separate items even when they
are plated touching each other, one resting on or against the other, or otherwise not spatially
separated into distinct piles. A minor garnish, condiment, or drizzle (e.g. a lemon wedge, a
sprig of herbs, a spoonful of sauce) that accompanies a main component rather than constituting
its own portion-sized food SHALL stay folded into the item it accompanies, even though it is
technically independently namable. This granularity judgment is made by the model on each photo;
it is not a user-facing setting and there is no per-upload or per-user toggle for it.

#### Scenario: A single composite dish is recognized as one item

- **WHEN** a photo shows a homogeneous composite dish, such as a mixed vegetable side, a
  stir-fry, or a sauced dish, where no individual ingredient is independently identifiable
- **THEN** Recognize returns exactly one item for that dish, named to reflect the dish as a
  whole, rather than one item per ingredient it may contain

#### Scenario: A plate with visually separate components is still decomposed

- **WHEN** a photo shows a plate with clearly separate components, such as a portion of rice, a
  piece of grilled protein, and a side salad plated apart from each other
- **THEN** Recognize returns one item per visually separate component, as it does today

#### Scenario: Independently identifiable foods plated touching each other are still split

- **WHEN** a photo shows two independently identifiable foods served touching or resting against
  each other rather than plated apart — such as a piece of white fish served on or next to a
  portion of stewed cabbage — where each remains visually distinguishable as its own food rather
  than blended into one preparation, whether or not the two foods share the same role (e.g. two
  different cooked vegetable sides touching are split just as a protein-and-side pairing is)
- **THEN** Recognize returns one item for each of the independently identifiable foods (e.g. one
  item for the fish, one item for the stewed cabbage), rather than merging them into a single
  named dish

#### Scenario: A minor garnish or condiment stays folded into its main component

- **WHEN** a photo shows a main food accompanied by a minor garnish, condiment, or drizzle — such
  as a lemon wedge, a sprig of herbs, or a spoonful of sauce — that is not itself a portion-sized
  food
- **THEN** Recognize returns one item for the main food and does not return a separate item for
  the garnish or condiment
