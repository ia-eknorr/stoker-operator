package git

import (
	"context"
	"fmt"
	"os"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// Result holds the outcome of a clone or fetch operation.
type Result struct {
	Commit string
	Ref    string
}

// Client is the interface for git operations used by the controller.
type Client interface {
	// CloneOrFetch clones the repo if the target directory is empty,
	// or fetches + checks out the ref if already cloned.
	// Returns the resolved commit SHA.
	CloneOrFetch(ctx context.Context, repoURL, ref, path string, auth transport.AuthMethod) (Result, error)
}

// GoGitClient implements Client using go-git.
type GoGitClient struct{}

var _ Client = (*GoGitClient)(nil)

func (g *GoGitClient) CloneOrFetch(ctx context.Context, repoURL, ref, path string, auth transport.AuthMethod) (Result, error) {
	// Check if the directory already contains a cloned repo
	if isCloned(path) {
		return g.fetchAndCheckout(ctx, ref, path, auth)
	}
	return g.cloneAndCheckout(ctx, repoURL, ref, path, auth)
}

func (g *GoGitClient) cloneAndCheckout(ctx context.Context, repoURL, ref, path string, auth transport.AuthMethod) (Result, error) {
	repo, err := gogit.PlainCloneContext(ctx, path, false, &gogit.CloneOptions{
		URL:  repoURL,
		Auth: auth,
	})
	if err != nil {
		return Result{}, fmt.Errorf("git clone %s: %w", repoURL, err)
	}

	return checkoutRef(repo, ref)
}

func (g *GoGitClient) fetchAndCheckout(ctx context.Context, ref, path string, auth transport.AuthMethod) (Result, error) {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return Result{}, fmt.Errorf("opening repo at %s: %w", path, err)
	}

	err = repo.FetchContext(ctx, &gogit.FetchOptions{
		Auth:  auth,
		Force: true,
		Tags:  gogit.AllTags,
	})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return Result{}, fmt.Errorf("git fetch: %w", err)
	}

	return checkoutRef(repo, ref)
}

// checkoutRef resolves a ref (branch, tag, or commit SHA) and checks it out.
func checkoutRef(repo *gogit.Repository, ref string) (Result, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return Result{}, fmt.Errorf("getting worktree: %w", err)
	}

	hash, err := resolveRef(repo, ref)
	if err != nil {
		return Result{}, err
	}

	if err := wt.Checkout(&gogit.CheckoutOptions{
		Hash:  hash,
		Force: true,
	}); err != nil {
		return Result{}, fmt.Errorf("checkout %s: %w", ref, err)
	}

	return Result{
		Commit: hash.String(),
		Ref:    ref,
	}, nil
}

// resolveRef tries to resolve a ref as: exact commit SHA, tag, then branch.
func resolveRef(repo *gogit.Repository, ref string) (plumbing.Hash, error) {
	// Try as a full SHA
	if plumbing.IsHash(ref) {
		return plumbing.NewHash(ref), nil
	}

	// Try as a tag
	tagRef, err := repo.Tag(ref)
	if err == nil {
		return tagRef.Hash(), nil
	}

	// Try as refs/tags/
	resolved, err := repo.ResolveRevision(plumbing.Revision("refs/tags/" + ref))
	if err == nil {
		return *resolved, nil
	}

	// Try as a branch (remote tracking)
	resolved, err = repo.ResolveRevision(plumbing.Revision("refs/remotes/origin/" + ref))
	if err == nil {
		return *resolved, nil
	}

	// Last resort: let go-git try to resolve it
	resolved, err = repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("cannot resolve ref %q: %w", ref, err)
	}
	return *resolved, nil
}

// isCloned checks if a directory contains a valid git repository.
func isCloned(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = gogit.PlainOpen(path)
	return err == nil
}
