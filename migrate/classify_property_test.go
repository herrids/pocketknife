package migrate

import (
	"testing"

	"pocketknife/schema"

	"pgregory.net/rapid"
)

// allOpKinds enumerates every member of the OpKind closed set, so the property
// generator below can sample any of them with equal weight.
var allOpKinds = []OpKind{
	OpAddEntity, OpDropEntity, OpRenameEntity,
	OpAddField, OpDropField, OpRenameField,
	OpChangeType, OpChangeRequired, OpChangeUnique, OpChangeEnum, OpChangeReference,
}

var allFieldTypes = []schema.FieldType{
	schema.TypeText, schema.TypeInteger, schema.TypeReal, schema.TypeBoolean,
	schema.TypeDatetime, schema.TypeEnum, schema.TypeReference,
}

// genField draws an arbitrary, possibly nonsensical *schema.Field: every member
// Classify might dereference for any OpKind is populated, regardless of whether
// the drawn Type would realistically carry that member (e.g. Values on a non-enum
// field). This is deliberate -- the property below asserts Classify's output is a
// pure function of operation structure, and proving that holds even over
// structurally-weird-but-non-nil fields is a stronger claim than restricting the
// generator to only "realistic" shapes a real Diff would ever emit.
func genField(t *rapid.T, label string) *schema.Field {
	f := &schema.Field{
		ID:         rapid.StringMatching(`fld_[a-z0-9]{1,8}`).Draw(t, label+"_id"),
		Name:       rapid.StringMatching(`[a-z][a-z0-9_]{0,10}`).Draw(t, label+"_name"),
		Type:       rapid.SampledFrom(allFieldTypes).Draw(t, label+"_type"),
		Required:   rapid.Bool().Draw(t, label+"_required"),
		Unique:     rapid.Bool().Draw(t, label+"_unique"),
		HasDefault: rapid.Bool().Draw(t, label+"_has_default"),
		Values:     rapid.SliceOfN(rapid.StringMatching(`[a-z]{1,6}`), 0, 5).Draw(t, label+"_values"),
		Target:     rapid.StringMatching(`ent_[a-z0-9]{1,8}`).Draw(t, label+"_target"),
		OnDelete: rapid.SampledFrom([]string{
			schema.OnDeleteSetNull, schema.OnDeleteRestrict, schema.OnDeleteCascade,
		}).Draw(t, label+"_ondelete"),
	}
	return f
}

func genEntity(t *rapid.T, label string) *schema.Entity {
	return &schema.Entity{
		ID:   rapid.StringMatching(`ent_[a-z0-9]{1,8}`).Draw(t, label+"_id"),
		Name: rapid.StringMatching(`[a-z][a-z0-9_]{0,10}`).Draw(t, label+"_name"),
	}
}

// genOperation draws a structurally-complete Operation: a random Kind, plus
// BeforeField/AfterField/BeforeEntity/AfterEntity all populated unconditionally
// (never nil), independent of whether that Kind's Classify branch would read
// them. Annotation is left at its zero value; the property test drives it
// separately.
func genOperation(t *rapid.T) Operation {
	return Operation{
		Kind:         rapid.SampledFrom(allOpKinds).Draw(t, "kind"),
		EntityID:     rapid.StringMatching(`ent_[a-z0-9]{1,8}`).Draw(t, "entity_id"),
		FieldID:      rapid.StringMatching(`fld_[a-z0-9]{1,8}`).Draw(t, "field_id"),
		BeforeField:  genField(t, "before_field"),
		AfterField:   genField(t, "after_field"),
		BeforeEntity: genEntity(t, "before_entity"),
		AfterEntity:  genEntity(t, "after_entity"),
	}
}

// TestPropertyClassifyIgnoresAnnotation generalizes the hand-picked
// TestAcceptanceMisAnnotationOverridden case: for an arbitrary Operation (any
// Kind, with every Before/After member populated, including combinations a real
// Diff would never produce), Classify's result must depend only on the
// operation's structure -- never on Annotation. A caller cannot talk the
// classifier into a different answer by lying about the class up front.
func TestPropertyClassifyIgnoresAnnotation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		op := genOperation(t)
		baseline := Classify(op)

		garbage := Class(rapid.String().Draw(t, "garbage_annotation"))
		for _, ann := range []Class{ClassUnclassified, ClassSafe, ClassDestructive, garbage} {
			op.Annotation = ann
			if got := Classify(op); got != baseline {
				t.Fatalf("Classify is not annotation-independent: kind=%s annotation=%q got=%q want=%q (op=%+v)",
					op.Kind, ann, got, baseline, op)
			}
		}
	})
}

// TestPropertyChangesetClassifyIgnoresAnnotation extends the same property to
// Changeset.Classify, which the real apply flow actually calls: running it over
// a batch of arbitrary, annotated operations must reproduce the exact same Class
// values that classifying each Operation in isolation would, regardless of the
// (ignored) Annotation each carries in.
func TestPropertyChangesetClassifyIgnoresAnnotation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 8).Draw(t, "op_count")
		ops := make([]Operation, n)
		want := make([]Class, n)
		for i := 0; i < n; i++ {
			op := genOperation(t)
			op.Annotation = Class(rapid.SampledFrom([]string{
				string(ClassSafe), string(ClassDestructive), "", "bogus",
			}).Draw(t, "annotation"))
			want[i] = Classify(op)
			ops[i] = op
		}

		cs := &Changeset{Ops: ops}
		cs.Classify()

		for i, op := range cs.Ops {
			if op.Class != want[i] {
				t.Fatalf("op %d: Changeset.Classify gave %q, want %q (annotation %q ignored as designed, kind=%s)",
					i, op.Class, want[i], ops[i].Annotation, op.Kind)
			}
		}
	})
}
