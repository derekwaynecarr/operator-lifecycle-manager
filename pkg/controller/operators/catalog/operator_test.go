package catalog

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	csvv1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/clusterserviceversion/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/installplan/v1alpha1"
)

type mockTransitioner struct {
	err error
}

var _ installPlanTransitioner = &mockTransitioner{}

func (m *mockTransitioner) ResolvePlan(plan *v1alpha1.InstallPlan) error {
	return m.err
}

func (m *mockTransitioner) ExecutePlan(plan *v1alpha1.InstallPlan) error {
	return m.err
}

func TestTransitionInstallPlan(t *testing.T) {
	var (
		errMsg = "transition test error"
		err    = errors.New(errMsg)

		resolved = &v1alpha1.InstallPlanCondition{
			Type:   v1alpha1.InstallPlanResolved,
			Status: corev1.ConditionTrue,
		}
		unresolved = &v1alpha1.InstallPlanCondition{
			Type:    v1alpha1.InstallPlanResolved,
			Status:  corev1.ConditionFalse,
			Reason:  v1alpha1.InstallPlanReasonInstallCheckFailed,
			Message: errMsg,
		}
		installed = &v1alpha1.InstallPlanCondition{
			Type:   v1alpha1.InstallPlanInstalled,
			Status: corev1.ConditionTrue,
		}
		failed = &v1alpha1.InstallPlanCondition{
			Type:    v1alpha1.InstallPlanInstalled,
			Status:  corev1.ConditionFalse,
			Reason:  v1alpha1.InstallPlanReasonComponentFailed,
			Message: errMsg,
		}
	)
	var table = []struct {
		initial    v1alpha1.InstallPlanPhase
		transError error
		approval   v1alpha1.Approval
		approved   bool
		expected   v1alpha1.InstallPlanPhase
		condition  *v1alpha1.InstallPlanCondition
	}{
		{v1alpha1.InstallPlanPhaseNone, nil, v1alpha1.ApprovalAutomatic, false, v1alpha1.InstallPlanPhasePlanning, nil},
		{v1alpha1.InstallPlanPhaseNone, nil, v1alpha1.ApprovalAutomatic, true, v1alpha1.InstallPlanPhasePlanning, nil},
		{v1alpha1.InstallPlanPhaseNone, err, v1alpha1.ApprovalAutomatic, false, v1alpha1.InstallPlanPhasePlanning, nil},
		{v1alpha1.InstallPlanPhaseNone, err, v1alpha1.ApprovalAutomatic, true, v1alpha1.InstallPlanPhasePlanning, nil},

		{v1alpha1.InstallPlanPhasePlanning, nil, v1alpha1.ApprovalAutomatic, false, v1alpha1.InstallPlanPhaseInstalling, resolved},
		{v1alpha1.InstallPlanPhasePlanning, nil, v1alpha1.ApprovalAutomatic, true, v1alpha1.InstallPlanPhaseInstalling, resolved},
		{v1alpha1.InstallPlanPhasePlanning, nil, v1alpha1.ApprovalManual, false, v1alpha1.InstallPlanPhaseRequiresApproval, resolved},
		{v1alpha1.InstallPlanPhasePlanning, nil, v1alpha1.ApprovalManual, true, v1alpha1.InstallPlanPhaseInstalling, resolved},
		{v1alpha1.InstallPlanPhasePlanning, err, v1alpha1.ApprovalAutomatic, false, v1alpha1.InstallPlanPhaseFailed, unresolved},
		{v1alpha1.InstallPlanPhasePlanning, err, v1alpha1.ApprovalAutomatic, true, v1alpha1.InstallPlanPhaseFailed, unresolved},

		{v1alpha1.InstallPlanPhaseInstalling, nil, v1alpha1.ApprovalAutomatic, false, v1alpha1.InstallPlanPhaseComplete, installed},
		{v1alpha1.InstallPlanPhaseInstalling, nil, v1alpha1.ApprovalAutomatic, true, v1alpha1.InstallPlanPhaseComplete, installed},
		{v1alpha1.InstallPlanPhaseInstalling, err, v1alpha1.ApprovalAutomatic, false, v1alpha1.InstallPlanPhaseFailed, failed},
		{v1alpha1.InstallPlanPhaseInstalling, err, v1alpha1.ApprovalAutomatic, true, v1alpha1.InstallPlanPhaseFailed, failed},

		{v1alpha1.InstallPlanPhaseRequiresApproval, nil, v1alpha1.ApprovalManual, false, v1alpha1.InstallPlanPhaseRequiresApproval, nil},
		{v1alpha1.InstallPlanPhaseRequiresApproval, nil, v1alpha1.ApprovalManual, true, v1alpha1.InstallPlanPhaseInstalling, nil},
	}
	for _, tt := range table {
		// Create a plan in the provided initial phase.
		plan := &v1alpha1.InstallPlan{
			Spec: v1alpha1.InstallPlanSpec{
				Approval: tt.approval,
				Approved: tt.approved,
			},
			Status: v1alpha1.InstallPlanStatus{
				Phase:      tt.initial,
				Conditions: []v1alpha1.InstallPlanCondition{},
			},
		}

		// Create a transitioner that returns the provided error.
		transitioner := &mockTransitioner{tt.transError}

		// Attempt to transition phases.
		transitionInstallPlanState(transitioner, plan)

		// Assert that the final phase is as expected.
		require.Equal(t, tt.expected, plan.Status.Phase)

		// Assert that the condition set is as expected
		if tt.condition == nil {
			require.Equal(t, 0, len(plan.Status.Conditions))
		} else {
			require.Equal(t, 1, len(plan.Status.Conditions))
			require.Equal(t, tt.condition.Type, plan.Status.Conditions[0].Type)
			require.Equal(t, tt.condition.Status, plan.Status.Conditions[0].Status)
			require.Equal(t, tt.condition.Reason, plan.Status.Conditions[0].Reason)
			require.Equal(t, tt.condition.Message, plan.Status.Conditions[0].Message)
		}
	}
}

func installPlan(names ...string) v1alpha1.InstallPlan {
	return v1alpha1.InstallPlan{
		Spec: v1alpha1.InstallPlanSpec{
			ClusterServiceVersionNames: names,
		},
		Status: v1alpha1.InstallPlanStatus{
			Plan: []v1alpha1.Step{},
		},
	}
}

func csv(name string, owned, required []string) csvv1alpha1.ClusterServiceVersion {
	requiredCRDDescs := make([]csvv1alpha1.CRDDescription, 0)
	for _, name := range required {
		requiredCRDDescs = append(requiredCRDDescs, csvv1alpha1.CRDDescription{Name: name, Version: "v1", Kind: name})
	}

	ownedCRDDescs := make([]csvv1alpha1.CRDDescription, 0)
	for _, name := range owned {
		ownedCRDDescs = append(ownedCRDDescs, csvv1alpha1.CRDDescription{Name: name, Version: "v1", Kind: name})
	}

	return csvv1alpha1.ClusterServiceVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: csvv1alpha1.ClusterServiceVersionSpec{
			CustomResourceDefinitions: csvv1alpha1.CustomResourceDefinitions{
				Owned:    ownedCRDDescs,
				Required: requiredCRDDescs,
			},
		},
	}
}

func crd(name string) v1beta1.CustomResourceDefinition {
	return v1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1beta1.CustomResourceDefinitionSpec{
			Group:   name + "group",
			Version: "v1",
			Names: v1beta1.CustomResourceDefinitionNames{
				Kind: name,
			},
		},
	}
}
