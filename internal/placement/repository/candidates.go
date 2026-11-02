package repository

import (
	"context"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
)

type Candidates interface {
	CandidateState(context.Context) (model.CandidateState, error)
}
