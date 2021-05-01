package link

import (
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ankitpokhrel/jira-cli/api"
	"github.com/ankitpokhrel/jira-cli/internal/cmdutil"
	"github.com/ankitpokhrel/jira-cli/internal/query"
	"github.com/ankitpokhrel/jira-cli/pkg/jira"
)

const (
	helpText     = `Link connects two issues to a given link type.`
	examples     = `$ jira issue link ISSUE-1 ISSUE-2 Duplicate`
	optionCancel = "Cancel"
)

// NewCmdLink is a link command.
func NewCmdLink() *cobra.Command {
	cmd := cobra.Command{
		Use:     "link INWARD_ISSUE_KEY OUTWARD_ISSUE_KEY ISSUE_LINK_TYPE",
		Short:   "Link connects two issues",
		Long:    helpText,
		Example: examples,
		Aliases: []string{"ln"},
		Annotations: map[string]string{
			"help:args": "INWARD_ISSUE_KEY\tIssue key of the source issue, eg: ISSUE-1\n" +
				"OUTWARD_ISSUE_KEY\tIssue key of the target issue, eg: ISSUE-2\n" +
				"ISSUE_LINK_TYPE\tRelationship between two issues, eg: Duplicates, Blocks etc.",
		},
		Run: link,
	}

	cmd.Flags().Bool("web", false, "Open inward issue in web browser after successful linking")

	return &cmd
}

func link(cmd *cobra.Command, args []string) {
	params := parseArgsAndFlags(args, cmd.Flags())
	client := api.Client(jira.Config{Debug: params.debug})
	lc := linkCmd{
		client:    client,
		linkTypes: nil,
		params:    params,
	}

	cmdutil.ExitIfError(lc.setInwardIssueKey())
	cmdutil.ExitIfError(lc.setOutwardIssueKey())
	cmdutil.ExitIfError(lc.setLinkTypes())
	cmdutil.ExitIfError(lc.setDesiredLinkType())

	if lc.params.linkType == optionCancel {
		fmt.Print("\033[0;31m✗\033[0m Action aborted\n")
		os.Exit(0)
	}

	lt, err := lc.verifyIssueLinkType()
	if err != nil {
		cmdutil.Errorf(err.Error())
		return
	}

	func() {
		s := cmdutil.Info("Linking issues")
		defer s.Stop()

		err := client.LinkIssue(lc.params.inwardIssueKey, lc.params.outwardIssueKey, lt.Name)
		cmdutil.ExitIfError(err)
	}()

	server := viper.GetString("server")

	fmt.Printf("\u001B[0;32m✓\u001B[0m Issues linked as \"%s\"\n", lc.params.linkType)
	fmt.Printf("%s/browse/%s\n", server, lc.params.inwardIssueKey)

	if web, _ := cmd.Flags().GetBool("web"); web {
		err := cmdutil.Navigate(server, lc.params.inwardIssueKey)
		cmdutil.ExitIfError(err)
	}
}

type linkParams struct {
	inwardIssueKey  string
	outwardIssueKey string
	linkType        string
	debug           bool
}

func parseArgsAndFlags(args []string, flags query.FlagParser) *linkParams {
	var inwardIssueKey, outwardIssueKey, linkType string

	nargs := len(args)
	if nargs >= 1 {
		inwardIssueKey = strings.ToUpper(args[0])
	}
	if nargs >= 2 {
		outwardIssueKey = args[1]
	}
	if nargs >= 3 {
		linkType = args[2]
	}

	debug, err := flags.GetBool("debug")
	cmdutil.ExitIfError(err)

	return &linkParams{
		inwardIssueKey:  inwardIssueKey,
		outwardIssueKey: outwardIssueKey,
		linkType:        linkType,
		debug:           debug,
	}
}

type linkCmd struct {
	client    *jira.Client
	linkTypes []*jira.IssueLinkType
	params    *linkParams
}

func (lc *linkCmd) setInwardIssueKey() error {
	if lc.params.inwardIssueKey != "" {
		return nil
	}

	var ans string

	qs := &survey.Question{
		Name:     "inwardIssueKey",
		Prompt:   &survey.Input{Message: "Inward issue key"},
		Validate: survey.Required,
	}
	if err := survey.Ask([]*survey.Question{qs}, &ans); err != nil {
		return err
	}
	lc.params.inwardIssueKey = ans

	return nil
}

func (lc *linkCmd) setOutwardIssueKey() error {
	if lc.params.outwardIssueKey != "" {
		return nil
	}

	var ans string

	qs := &survey.Question{
		Name:     "outwardIssueKey",
		Prompt:   &survey.Input{Message: "Outward issue key"},
		Validate: survey.Required,
	}
	if err := survey.Ask([]*survey.Question{qs}, &ans); err != nil {
		return err
	}
	lc.params.outwardIssueKey = ans

	return nil
}

func (lc *linkCmd) setLinkTypes() error {
	s := cmdutil.Info("Fetching link types. Please wait...")
	defer s.Stop()

	lt, err := lc.client.GetIssueLinkTypes()
	if err != nil {
		return err
	}
	lc.linkTypes = lt

	return nil
}

func (lc *linkCmd) setDesiredLinkType() error {
	if lc.params.linkType != "" {
		return nil
	}

	options := make([]string, 0, len(lc.linkTypes))
	for _, t := range lc.linkTypes {
		options = append(options, fmt.Sprintf("%s: %s", t.Name, t.Inward))
	}
	options = append(options, optionCancel)

	var ans string

	qs := &survey.Question{
		Name: "linkType",
		Prompt: &survey.Select{
			Message: "Link type:",
			Options: options,
		},
		Validate: survey.Required,
	}
	if err := survey.Ask([]*survey.Question{qs}, &ans); err != nil {
		return err
	}
	lc.params.linkType = strings.Split(ans, ":")[0]

	return nil
}

func (lc *linkCmd) verifyIssueLinkType() (*jira.IssueLinkType, error) {
	var lt *jira.IssueLinkType

	st := strings.ToLower(lc.params.linkType)
	all := make([]string, 0, len(lc.linkTypes))
	for _, t := range lc.linkTypes {
		if strings.ToLower(t.Name) == st {
			lt = t
		}
		all = append(all, fmt.Sprintf("'%s'", t.Name))
	}

	if lt == nil {
		return nil, fmt.Errorf(
			"\u001B[0;31m✗\u001B[0m Invalid issue link type \"%s\"\nAvailable issue link types are: %s",
			lc.params.linkType, strings.Join(all, ", "),
		)
	}
	return lt, nil
}
