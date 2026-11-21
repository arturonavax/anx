import { bindings, defineConfig } from "cf/config";

export default defineConfig({
	worker: {
		name: "env-installer-router",
		compatibilityDate: "2024-10-31",
		compatibilityFlags: [
			"nodejs_compat",
		],
		entrypoint: "src/index.ts",
		domains: [
			"sh.arturonavax.dev",
		],
		env: {
			GITHUB_USER: bindings.text("arturonavax"),
			GITHUB_REPO: bindings.text("anx"),
			DEFAULT_BRANCH: bindings.text("main"),
			PERSONAL_WEBSITE_URL: bindings.text("https://arturonavax.dev/env"),
		},
	},
});
