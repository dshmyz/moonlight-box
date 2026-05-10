package service

import (
	"context"
	"fmt"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/types"
)

type DownloadService struct {
	repoRepo  *repository.RepositoryRepository
	groupRepo *repository.GroupRepository
	adapters  map[string]types.Adapter
}

func NewDownloadService(
	repoRepo *repository.RepositoryRepository,
	groupRepo *repository.GroupRepository,
	adapters map[string]types.Adapter,
) *DownloadService {
	return &DownloadService{
		repoRepo:  repoRepo,
		groupRepo: groupRepo,
		adapters:  adapters,
	}
}

func (s *DownloadService) Download(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
	switch downloadCtx.Repo.Type {
	case model.RepoTypeLocal:
		return s.downloadFromLocal(ctx, downloadCtx)
	case model.RepoTypeProxy:
		return s.downloadFromProxy(ctx, downloadCtx)
	case model.RepoTypeVirtual:
		return s.downloadFromVirtual(ctx, downloadCtx)
	default:
		return nil, fmt.Errorf("unsupported repository type: %s", downloadCtx.Repo.Type)
	}
}

func (s *DownloadService) downloadFromLocal(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
	adp := s.adapters[string(downloadCtx.PkgType)]
	if adp == nil {
		return nil, fmt.Errorf("unsupported package type: %s", downloadCtx.PkgType)
	}

	return adp.HandleDownload(nil, downloadCtx)
}

func (s *DownloadService) downloadFromProxy(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
	adp := s.adapters[string(downloadCtx.PkgType)]
	if adp == nil {
		return nil, fmt.Errorf("unsupported package type: %s", downloadCtx.PkgType)
	}

	return adp.HandleDownload(nil, downloadCtx)
}

func (s *DownloadService) downloadFromVirtual(ctx context.Context, downloadCtx *types.DownloadContext) (*types.DownloadResult, error) {
	members, err := s.groupRepo.GetMembersByVirtualRepo(downloadCtx.Repo.ID)
	if err != nil {
		return nil, err
	}

	for _, member := range members {
		if string(member.MemberRepo.PackageType) != string(downloadCtx.PkgType) {
			continue
		}

		memberCtx := *downloadCtx
		memberCtx.Repo = &member.MemberRepo

		result, err := s.Download(ctx, &memberCtx)
		if err == nil {
			result.RepoID = member.MemberRepo.ID
			return result, nil
		}
	}

	return nil, fmt.Errorf("package not found in virtual repository")
}
