package sandbox

import (
	"testing"
)

func TestGenerateUniqueFlag(t *testing.T) {
	baseFlag := "secret_level_1_base_flag"
	student1 := "2021170034"
	student2 := "2021170035"

	flag1 := GenerateUniqueFlag(baseFlag, student1)
	flag2 := GenerateUniqueFlag(baseFlag, student1)
	flag3 := GenerateUniqueFlag(baseFlag, student2)

	if len(flag1) != 16 {
		t.Errorf("Expected flag length 16, got %d (%s)", len(flag1), flag1)
	}

	if flag1 != flag2 {
		t.Errorf("Expected deterministic flags for same student, got %s != %s", flag1, flag2)
	}

	if flag1 == flag3 {
		t.Errorf("Expected different flags for different student IDs, both got %s", flag1)
	}
}
