package tools

func registerRepoTools(reg *Registry) {
	reg.Register(NewListDirTool())
	reg.Register(NewReadFileTool(1 << 20))
	reg.Register(NewGrepSearchTool(1 << 20))
	reg.Register(NewApplyPatchTool(30))
	reg.Register(NewWriteNewFileTool())
	reg.Register(NewDeletePathTool())
	reg.Register(NewMovePathTool())
	reg.Register(NewGitStatusTool(30, 64<<10))
	reg.Register(NewGitDiffTool(30, 64<<10))
	reg.Register(NewGitShowTool(30, 64<<10))
	reg.Register(NewGitLogTool(30, 64<<10))
	reg.Register(NewShellExecTool(30, 64<<10))
	reg.Register(NewRunTestsTool(60, 64<<10))
	reg.Register(NewRunBuildTool(60, 64<<10))
}
