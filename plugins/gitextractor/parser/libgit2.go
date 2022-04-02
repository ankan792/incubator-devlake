package parser

import (
	"context"
	"fmt"

	git "github.com/libgit2/git2go/v33"
	"github.com/merico-dev/lake/logger"
	"github.com/merico-dev/lake/models/domainlayer"
	"github.com/merico-dev/lake/models/domainlayer/code"
	"github.com/merico-dev/lake/plugins/gitextractor/models"
)

const (
	BRANCH = "BRANCH"
	TAG    = "TAG"
)

type LibGit2 struct {
	store models.Store
}

func NewLibGit2(store models.Store) *LibGit2 {
	return &LibGit2{store: store}
}

func (l *LibGit2) LocalRepo(ctx context.Context, repoPath, repoId string) error {
	repo, err := git.OpenRepository(repoPath)
	if err != nil {
		return err
	}
	return l.run(ctx, repo, repoId)
}

func (l *LibGit2) run(ctx context.Context, repo *git.Repository, repoId string) error {
	defer l.store.Close()

	// collect tags
	var err error
	err = repo.Tags.Foreach(func(name string, id *git.Oid) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			break
		}
		var err1 error
		var obj *git.Object
		var tag *git.Tag
		obj, err1 = repo.Lookup(id)
		if err1 != nil {
			return err1
		}
		var tagCommit string
		tag, _ = obj.AsTag()
		if tag != nil {
			tagCommit = tag.TargetId().String()
		} else {
			tagCommit = id.String()
		}
		if tagCommit != "" {
			ref := &code.Ref{
				DomainEntity: domainlayer.DomainEntity{Id: fmt.Sprintf("%s:%s", repoId, name)},
				RepoId:       repoId,
				Ref:          name,
				CommitSha:    tagCommit,
				RefType:      TAG,
			}
			err1 = l.store.Refs(ref)
			if err1 != nil {
				return err1
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// collect branches
	var repoInter *git.BranchIterator
	repoInter, err = repo.NewBranchIterator(git.BranchAll)
	if err != nil {
		return err
	}
	err = repoInter.ForEach(func(branch *git.Branch, branchType git.BranchType) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			break
		}
		if branch.IsBranch() {
			name, err1 := branch.Name()
			if err1 != nil {
				return err1
			}
			var sha string
			if oid := branch.Target(); oid != nil {
				sha = oid.String()
			}
			ref := &code.Ref{
				DomainEntity: domainlayer.DomainEntity{Id: fmt.Sprintf("%s:%s", repoId, name)},
				RepoId:       repoId,
				Ref:          name,
				CommitSha:    sha,
				RefType:      BRANCH,
			}
			ref.IsDefault, _ = branch.IsHead()
			err1 = l.store.Refs(ref)
			if err1 != nil {
				return err1
			}
			return nil
		}
		return nil
	})
	if err != nil {
		return err
	}

	// collect commits
	opts, err := git.DefaultDiffOptions()
	if err != nil {
		return err
	}
	opts.NotifyCallback = func(diffSoFar *git.Diff, delta git.DiffDelta, matchedPathSpec string) error {
		return nil
	}
	revWalk, err := repo.Walk()
	if err != nil {
		return err
	}
	err = revWalk.PushHead()
	if err != nil {
		return err
	}
	var err2 error
	err = revWalk.Iterate(func(commit *git.Commit) bool {
		commitSha := commit.Id().String()
		logger.Info("process commit:", commitSha)
		select {
		case <-ctx.Done():
			err2 = ctx.Err()
			return false
		default:
			break
		}
		c := &code.Commit{
			Sha:     commitSha,
			Message: commit.Message(),
		}
		author := commit.Author()
		if author != nil {
			c.AuthorName = author.Name
			c.AuthorEmail = author.Email
			c.AuthorId = author.Email
			c.AuthoredDate = author.When
		}
		committer := commit.Committer()
		if committer != nil {
			c.CommitterName = committer.Name
			c.CommitterEmail = committer.Email
			c.CommitterId = committer.Email
			c.CommittedDate = committer.When
		}
		var commitParents []*code.CommitParent
		for i := uint(0); i < commit.ParentCount(); i++ {
			parent := commit.Parent(i)
			if parent != nil {
				if parentId := parent.Id(); parentId != nil {
					commitParents = append(commitParents, &code.CommitParent{
						CommitSha:       c.Sha,
						ParentCommitSha: parentId.String(),
					})
				}
			}
		}
		err2 = l.store.CommitParents(commitParents)
		if err2 != nil {
			return false
		}
		if commit.ParentCount() > 0 {
			parent := commit.Parent(0)
			if parent != nil {
				var parentTree, tree *git.Tree
				parentTree, err2 = parent.Tree()
				if err2 != nil {
					return false
				}
				tree, err2 = commit.Tree()
				if err2 != nil {
					return false
				}
				var diff *git.Diff
				diff, err2 = repo.DiffTreeToTree(parentTree, tree, &opts)
				if err2 != nil {
					return false
				}
				var commitFile *code.CommitFile
				err2 = diff.ForEach(func(file git.DiffDelta, progress float64) (
					git.DiffForEachHunkCallback, error) {
					if commitFile != nil {
						err2 = l.store.CommitFiles(commitFile)
						if err2 != nil {
							logger.Error("CommitFiles error:", err2)
							return nil, err2
						}
					}
					commitFile = new(code.CommitFile)
					commitFile.CommitSha = c.Sha
					commitFile.FilePath = file.NewFile.Path
					return func(hunk git.DiffHunk) (git.DiffForEachLineCallback, error) {
						return func(line git.DiffLine) error {
							if line.Origin == git.DiffLineAddition {
								commitFile.Additions += line.NumLines
							}
							if line.Origin == git.DiffLineDeletion {
								commitFile.Deletions += line.NumLines
							}
							return nil
						}, nil
					}, nil
				}, git.DiffDetailLines)
				if err2 != nil {
					return false
				}
				if commitFile != nil {
					err2 = l.store.CommitFiles(commitFile)
					if err2 != nil {
						logger.Error("CommitFiles error:", err2)
					}
				}
				var stats *git.DiffStats
				stats, err2 = diff.Stats()
				if err2 != nil {
					return false
				}
				c.Additions += stats.Insertions()
				c.Deletions += stats.Deletions()
			}
		}
		err2 = l.store.Commits(c)
		if err2 != nil {
			return false
		}
		repoCommit := &code.RepoCommit{
			RepoId:    repoId,
			CommitSha: c.Sha,
		}
		err2 = l.store.RepoCommits(repoCommit)
		if err2 != nil {
			return false
		}
		return true
	})
	if err2 != nil {
		return err2
	}
	if err == nil {
		err = l.store.Flush()
	}
	return err
}
