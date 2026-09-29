package cli

import (
	"fmt"
	"strings"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "group",
		Aliases: []string{"profile"},
		Short:   "Manage custom skill groups for batch deployment",
	}

	cmd.AddCommand(newGroupListCmd())
	cmd.AddCommand(newGroupCreateCmd())
	cmd.AddCommand(newGroupDeleteCmd())
	cmd.AddCommand(newGroupAddCmd())
	cmd.AddCommand(newGroupRemoveCmd())

	return cmd
}

func newGroupListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all skill groups and their assigned skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			groups, err := appService.GroupList()
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(groups)
			}

			if len(groups) == 0 {
				out.Infof(i18n.T("cli.group.none"))
				return nil
			}

			out.Println()
			out.Printf(i18n.T("cli.group.title"), len(groups))
			for name, g := range groups {
				skillsStr := "none"
				if len(g.Skills) > 0 {
					skillsStr = strings.Join(g.Skills, ", ")
				}
				out.Printf(i18n.T("cli.group.item"), name, len(g.Skills), skillsStr)
			}
			out.Println()
			return nil
		},
	}
	return cmd
}

func newGroupCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <group-name>",
		Short: "Create a new skill group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := appService.GroupCreate(name); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status": "created",
					"group":  name,
				})
			}

			out.Successf(i18n.T("cli.group.created"), name)
			return nil
		},
	}
	return cmd
}

func newGroupDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <group-name>",
		Short: "Delete a skill group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := appService.GroupDelete(name); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status": "deleted",
					"group":  name,
				})
			}

			out.Successf(i18n.T("cli.group.deleted"), name)
			return nil
		},
	}
	return cmd
}

func newGroupAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <group-name> <skill-id...>",
		Short: "Add one or more skills to a group",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			skillIDs := args[1:]

			failed := make(map[string]string)
			for _, id := range skillIDs {
				if err := appService.GroupAddSkill(name, id); err != nil {
					failed[id] = err.Error()
				}
			}

			if out.json {
				if err := out.PrintJSON(map[string]interface{}{
					"status": "skills_added",
					"group":  name,
					"skills": skillIDs,
					"failed": failed,
				}); err != nil {
					return err
				}
				if len(failed) > 0 {
					return fmt.Errorf("failed to add %d skill(s) to group %q", len(failed), name)
				}
				return nil
			}

			for id, fErr := range failed {
				out.Errorf(i18n.T("cli.group.add_failed"), id, fErr)
			}
			if len(failed) > 0 {
				return fmt.Errorf("failed to add %d skill(s) to group %q", len(failed), name)
			}
			out.Successf(i18n.T("cli.group.added"), strings.Join(skillIDs, ", "), name)
			return nil
		},
	}
	return cmd
}

func newGroupRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <group-name> <skill-id...>",
		Short: "Remove one or more skills from a group",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			skillIDs := args[1:]

			failed := make(map[string]string)
			for _, id := range skillIDs {
				if err := appService.GroupRemoveSkill(name, id); err != nil {
					failed[id] = err.Error()
				}
			}

			if out.json {
				if err := out.PrintJSON(map[string]interface{}{
					"status": "skills_removed",
					"group":  name,
					"skills": skillIDs,
					"failed": failed,
				}); err != nil {
					return err
				}
				if len(failed) > 0 {
					return fmt.Errorf("failed to remove %d skill(s) from group %q", len(failed), name)
				}
				return nil
			}

			for id, fErr := range failed {
				out.Errorf(i18n.T("cli.group.remove_failed"), id, fErr)
			}
			if len(failed) > 0 {
				return fmt.Errorf("failed to remove %d skill(s) from group %q", len(failed), name)
			}
			out.Successf(i18n.T("cli.group.removed"), strings.Join(skillIDs, ", "), name)
			return nil
		},
	}
	return cmd
}
