# Backlog Task Templates

This directory contains standardized templates for common work patterns in the Loqa project. These templates ensure consistency, completeness, and proper context for different types of tasks.

## Available Templates

### 1. General Task Template (`general-task-template.md`)
**Use for:** Standard tasks, improvements, documentation, or any work that doesn't fit other specific patterns.

**Key Features:**
- Comprehensive acceptance criteria
- Component impact tracking
- Quality gates and definition of done

### 2. Bug Fix Template (`bug-fix-template.md`)
**Use for:** Fixing bugs, addressing issues, or resolving defects.

**Key Features:**
- Structured problem analysis
- Root cause investigation
- Rollback planning
- Regression testing requirements

### 3. Feature Template (`feature-template.md`)
**Use for:** New features, enhancements, or significant functionality additions.

**Key Features:**
- User story format
- Technical design section
- Launch planning
- Success metrics

### 4. Protocol Change Template (`protocol-change-template.md`)
**Use for:** Changes to gRPC protocols, API modifications, or service interface updates.

**Key Features:**
- Backward compatibility tracking
- Service coordination
- Migration planning
- Version management

### 5. Cross-Repository Work Template (`cross-repo-work-template.md`)
**Use for:** Work that spans multiple repositories requiring coordination.

**Key Features:**
- Dependency ordering
- Branch coordination strategy
- Quality gate sequencing
- Integration testing plan

## How to Use Templates

### Method 1: Manual Copy
1. Copy the appropriate template file
2. Rename to your task: `task-001-your-task-name.md`
3. Fill in all sections with specific details
4. Save in `backlog/tasks/` directory

### Method 2: Backlog.md CLI
```bash
# Create task using backlog CLI
backlog task create "Your task title"

# Then copy template content into the created task file
```

### Method 3: Interactive Commands (Future)
Once MCP commands are implemented:
```bash
/add-todo --template=feature "New voice command processing"
/add-todo --template=bug-fix "Audio streaming disconnection issue"
```

## Template Guidelines

### Required Sections
All templates include these mandatory sections:
- **Overview/Description:** Clear statement of what needs to be done
- **Acceptance Criteria:** Specific, measurable outcomes
- **Quality Gates:** Code review, testing, and quality requirements
- **Related Issues:** Links to GitHub issues and dependencies

### Component Tracking
Each template includes checkboxes for affected components:
- loqa-hub (Central service)
- loqa-commander (Admin dashboard)
- loqa-relay (Audio client)
- loqa-proto (Protocol definitions)
- loqa-skills (Plugin system)
- www-loqalabs-com (Website)

### Quality Standards
All templates enforce these quality requirements:
- Code review completion
- Test coverage maintenance
- Quality check execution (`make quality-check`)
- Documentation updates

## Best Practices

### When Creating Tasks
1. **Choose the right template** based on work type
2. **Fill in all sections** - don't skip context or acceptance criteria
3. **Be specific** in acceptance criteria and implementation approach
4. **Consider dependencies** and coordination needs
5. **Plan for testing** and quality validation

### Template Customization
- **Add project-specific sections** as needed
- **Modify checklists** to match your workflow
- **Include additional quality gates** for your environment
- **Adapt acceptance criteria** format to your needs

### Cross-Repository Coordination
For work affecting multiple repositories:
1. Use the Cross-Repository Work Template
2. Create matching feature branches
3. Coordinate quality checks and testing
4. Plan deployment order carefully

## Integration with Workflow

### GitHub Issues Integration
- Link backlog tasks to GitHub issues
- Use GitHub issue numbers in task IDs when possible
- Sync status between GitHub and backlog.md

### Branch Strategy
- Create feature branches matching task names
- Use consistent naming: `feature/task-001-description`
- Coordinate across repositories for cross-repo work

### Quality Integration
- Run quality checks before marking tasks complete
- Use `make quality-check` for Go services
- Use `npm run quality-check` for JavaScript services
- Ensure all tests pass before task completion

## Future Enhancements

### Planned MCP Commands
- `/add-todo --template=<type>` for interactive task creation
- `/capture-thought` for quick idea capture
- `/set-role` for specialized work contexts

### Template Evolution
Templates will be updated based on:
- Team feedback and usage patterns
- New work types and requirements
- Quality process improvements
- Tool integration opportunities