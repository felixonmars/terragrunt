package scaffold

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terratest/modules/files"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	boilerplate_options "github.com/gruntwork-io/boilerplate/options"
	"github.com/gruntwork-io/boilerplate/templates"
	"github.com/gruntwork-io/boilerplate/variables"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/go-getter"
)

const (
	defaultBoilerplateTemplate = `
# This is a Terragrunt module generated by boilerplate.
terraform {
  # ${.moduleUrl}
  source = "."
}

inputs = {
  {{range .parsedInputs}}
  {{.}} = ""
  {{end}}
}
`
)

func Run(opts *options.TerragruntOptions) error {
	// download remote repo to local
	moduleUrl := ""
	templateUrl := ""
	if len(opts.TerraformCliArgs) >= 2 {
		moduleUrl = opts.TerraformCliArgs[1]
	}

	if len(opts.TerraformCliArgs) >= 3 {
		templateUrl = opts.TerraformCliArgs[2]
	}

	tempDir, err := os.MkdirTemp("", "scaffold")
	if err != nil {
		return errors.WithStackTrace(err)
	}

	opts.Logger.Infof("Scaffolding a new Terragrunt module %s %s to %s", moduleUrl, templateUrl, opts.WorkingDir)

	err = getter.GetAny(tempDir, moduleUrl)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	err = files.CopyFolderContents(tempDir, opts.WorkingDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	inputs, err := listInputs(opts, opts.WorkingDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// run boilerplate

	// prepare boilerplate dir
	boilerplateDir := util.JoinPath(opts.WorkingDir, util.DefaultBoilerplateDir)
	if !files.IsExistingDir(boilerplateDir) {
		// no default boilerplate dir, create one
		boilerplateDir, err = os.MkdirTemp("", "scaffold")
		if err != nil {
			return errors.WithStackTrace(err)
		}
		err = os.WriteFile(util.JoinPath(boilerplateDir, "terragrunt.hcl"), []byte(defaultBoilerplateTemplate), 0644)
	}

	// prepare inputs
	vars, err := variables.ParseVars(opts.ScaffoldVars, opts.ScaffoldVarFiles)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	vars["parsedInputs"] = inputs
	vars["moduleUrl"] = moduleUrl

	opts.Logger.Infof("Running boilerplate in %s", opts.WorkingDir)
	boilerplateOpts := &boilerplate_options.BoilerplateOptions{
		TemplateFolder:  boilerplateDir,
		OutputFolder:    opts.WorkingDir,
		OnMissingKey:    boilerplate_options.DefaultMissingKeyAction,
		OnMissingConfig: boilerplate_options.DefaultMissingConfigAction,
		Vars:            vars,

		NonInteractive: true,
	}
	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(boilerplateOpts, boilerplateOpts, emptyDep)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// running fmt
	err = hclfmt.Run(opts)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func listInputs(opts *options.TerragruntOptions, directoryPath string) ([]string, error) {
	tfFiles, err := listTerraformFiles(directoryPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	parser := hclparse.NewParser()

	// Extract variables from all TF files
	var foundVars []string
	for _, tfFile := range tfFiles {
		content, err := os.ReadFile(tfFile)
		if err != nil {
			opts.Logger.Errorf("Error reading file %s: %v", tfFile, err)
			continue
		}
		file, diags := parser.ParseHCL(content, tfFile)
		if diags.HasErrors() {
			opts.Logger.Warnf("Failed to parse HCL in file %s: %v", tfFile, diags)
			continue
		}
		if body, ok := file.Body.(*hclsyntax.Body); ok {
			for _, block := range body.Blocks {
				if block.Type == "variable" {
					if len(block.Labels[0]) > 0 {
						foundVars = append(foundVars, block.Labels[0])
					}
				}
			}
		}
	}
	return foundVars, nil
}

// listTerraformFiles returns a list of all TF files in the specified directory.
func listTerraformFiles(directoryPath string) ([]string, error) {
	var tfFiles []string

	err := filepath.Walk(directoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".tf" {
			tfFiles = append(tfFiles, path)
		}
		return nil
	})

	return tfFiles, err
}