package wrapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func nodeRepo() map[string]string {
	return map[string]string{
		"package.json": `{"name": "root", "workspaces": ["packages/*", "!packages/skipped", "../outside/*"],
			"scripts": {"prelint": "echo pre", "lint": "eslint . && npm run types", "postlint": "echo post",
				"types": "tsc --noEmit", "test": "vitest run", "loop": "npm run loop", "ci": "turbo run check"}}`,
		"packages/web/package.json": `{"name": "@acme/web", "scripts": {"lint": "next lint", "check": "biome check ."}}`,
		"packages/api/package.json": `{"name": "api", "scripts": {"lint": "oxlint", "test": "jest"}}`,
		"packages/skipped/package.json": `{"name": "skipped", "scripts": {"lint": "never"}}`,
		"packages/notes/README.md":      "not a package",
	}
}

func TestNpmRunsPackageScripts(t *testing.T) {
	files := nodeRepo()
	assert.Equal(t, []string{"echo pre", "eslint . && npm run types", "tsc --noEmit", "echo post"}, expand(t, files, "npm run lint -- --fix"))
	assert.Equal(t, []string{"vitest run"}, expand(t, files, "npm test"))
	assert.Equal(t, []string{"next lint"}, expand(t, files, "npm run lint -w @acme/web"))
	assert.Equal(t, []string{"oxlint"}, expand(t, files, "npm run lint --workspace=packages/api"))
	assert.ElementsMatch(t, []string{"next lint", "oxlint"}, expand(t, files, "npm run lint --workspaces --if-present"))
	assert.Equal(t, []string{"next lint"}, expand(t, files, "npm --prefix packages/web run lint"))
	assert.Equal(t, []string{"npm run loop"}, expand(t, files, "npm run loop"), "a script that calls itself stops")
	assert.Empty(t, expand(t, files, "npm ci && npm run missing && npm"))
}

func TestYarnPnpmAndBunRunScriptsOrPrograms(t *testing.T) {
	files := nodeRepo()
	files["pnpm-workspace.yaml"] = "packages:\n  - 'packages/**'\n"
	assert.Equal(t, []string{"vitest run"}, expand(t, files, "yarn test"))
	assert.Equal(t, []string{"next lint"}, expand(t, files, "yarn workspace @acme/web lint"))
	assert.ElementsMatch(t, []string{"next lint", "oxlint"}, expand(t, files, "yarn workspaces foreach -A --exclude root run lint"))
	assert.ElementsMatch(t, []string{"jest"}, expand(t, files, "yarn workspaces run test"))
	assert.Empty(t, expand(t, files, "yarn install --frozen-lockfile && yarn && yarn run"))
	assert.Equal(t, []string{"biome check ."}, expand(t, files, "yarn turbo run check"), "with no such script, the program")

	assert.ElementsMatch(t, []string{"next lint", "oxlint"}, expand(t, files, "pnpm -r lint"))
	assert.ElementsMatch(t, []string{"next lint", "oxlint"}, expand(t, files, "pnpm --filter ./packages/** run lint"))
	assert.Equal(t, []string{"jest"}, expand(t, files, "pnpm -C packages/api test"))
	assert.Equal(t, []string{"eslint . && npm run types", "tsc --noEmit"}, expand(t, files, "pnpm lint"), "no pre and post scripts")
	assert.Empty(t, expand(t, files, "pnpm install && pnpm"))

	assert.Equal(t, []string{"oxlint"}, expand(t, files, "bun --cwd packages/api run lint"))
	assert.ElementsMatch(t, []string{"next lint", "oxlint"}, expand(t, files, "bun --filter '*' lint"))
	assert.Empty(t, expand(t, files, "bun test && bun"))
}

func TestMonorepoRunnersRunWorkspaceScripts(t *testing.T) {
	files := nodeRepo()
	files["package.json"] = `{"workspaces": {"packages": ["packages/*"]}, "scripts": {"ci": "turbo run check"}}`
	delete(files, "packages/skipped/package.json")
	assert.ElementsMatch(t, []string{"next lint", "oxlint", "jest"}, expand(t, files, "npx turbo run lint test --filter=...[origin/main]"))
	assert.Equal(t, []string{"biome check ."}, expand(t, files, "turbo check"))
	assert.Equal(t, []string{"turbo run check", "biome check ."}, expand(t, files, "corepack yarn ci"))
	assert.ElementsMatch(t, []string{"next lint", "oxlint"}, expand(t, files, "lerna run lint --scope api"))
	assert.ElementsMatch(t, []string{"next lint", "oxlint", "jest"}, expand(t, files, "nx run-many -t lint,test --parallel"))
	assert.ElementsMatch(t, []string{"next lint", "oxlint"}, expand(t, files, "pnpm exec nx affected --target=lint --base=origin/main"))
	assert.Equal(t, []string{"next lint"}, expand(t, files, "nx run @acme/web:lint:ci"))
	assert.Equal(t, []string{"oxlint"}, expand(t, files, "nx lint api"))
	assert.Equal(t, []string{"oxlint"}, expand(t, files, "nx api:lint"))
	assert.Empty(t, expand(t, files, "nx graph && nx run && nx && lerna bootstrap && turbo"))
	assert.Equal(t, []string{"biome check ."}, expand(t, files, "npx -y turbo check"))
}

func TestPackageScriptsWithoutAPackage(t *testing.T) {
	assert.Empty(t, expand(t, map[string]string{"package.json": "not json"}, "npm run lint && yarn workspaces run lint"))
	assert.Empty(t, expand(t, map[string]string{"pnpm-workspace.yaml": ": not yaml"}, "pnpm -r lint && yarn workspace web lint"))
}
