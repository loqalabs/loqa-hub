# Cross-Repository Work Template

## Work Overview
**Work Type:** [Feature/Bug Fix/Refactoring/Migration/Integration]

**Scope:** [Brief description of the cross-repository work]

**Lead Repository:** [Primary repository driving the work]

**Impact Level:** [Low/Medium/High/Critical]

## Affected Repositories
- [ ] loqa (Main orchestration)
- [ ] loqa-hub (Central service)
- [ ] loqa-commander (Admin dashboard)
- [ ] loqa-relay (Audio client)
- [ ] loqa-proto (Protocol definitions)
- [ ] loqa-skills (Plugin system)
- [ ] www-loqalabs-com (Website)
- [ ] loqalabs-github-config (GitHub templates)

## Coordination Requirements
**Dependency Order:** [List the order in which repositories must be updated]
1. [Repository 1] - [Reason]
2. [Repository 2] - [Reason]
3. [Repository 3] - [Reason]

**Breaking Changes:** [Yes/No - If yes, describe coordination strategy]

**Synchronized Release:** [Yes/No - If yes, describe release strategy]

## Work Breakdown by Repository

### Repository: [repo-name]
**Branch:** feature/[branch-name]
**Responsible Developer:** [name]
**Changes:**
- [ ] [Specific change 1]
- [ ] [Specific change 2]
- [ ] [Specific change 3]

**Dependencies:**
- Depends on: [other repo changes]
- Blocks: [other repo changes]

### Repository: [repo-name-2]
**Branch:** feature/[branch-name]
**Responsible Developer:** [name]
**Changes:**
- [ ] [Specific change 1]
- [ ] [Specific change 2]

## Feature Branch Strategy
**Branch Naming:** `feature/[issue-number]-[description]`

**Coordination Branches:**
- [ ] Create matching feature branches across all affected repos
- [ ] Use git worktrees for parallel development (if needed)
- [ ] Coordinate branch updates and merges

## Integration Testing Plan
- [ ] Individual repository testing
- [ ] Cross-repository integration testing
- [ ] End-to-end system testing
- [ ] Docker Compose testing with all services

## Quality Gates Coordination
**Quality Check Order:**
1. [ ] loqa-proto: `make quality-check`
2. [ ] loqa-skills: `make quality-check`
3. [ ] loqa-hub: `make quality-check`
4. [ ] loqa-relay: `make quality-check`
5. [ ] loqa-commander: `npm run quality-check`
6. [ ] Integration testing: `make test` from loqa/

## Deployment Coordination
**Deployment Order:**
1. [ ] [Repository 1] - [Deployment notes]
2. [ ] [Repository 2] - [Deployment notes]
3. [ ] [Repository 3] - [Deployment notes]

**Rollback Strategy:**
- [ ] Rollback order defined (reverse of deployment)
- [ ] Rollback tested in development
- [ ] Emergency rollback contacts identified

## Communication Plan
- [ ] Initial announcement to team
- [ ] Regular progress updates
- [ ] Coordination meetings scheduled
- [ ] Final completion announcement

## Pull Request Strategy
**PR Creation Order:**
1. [ ] [Repository 1] - Draft PR for visibility
2. [ ] [Repository 2] - Draft PR for visibility
3. [ ] [Repository 3] - Draft PR for visibility

**PR Review Process:**
- [ ] Cross-repository reviews completed
- [ ] Integration testing results shared
- [ ] All PRs ready before any merges

## Monitoring and Validation
- [ ] Health checks for all affected services
- [ ] Integration monitoring configured
- [ ] Performance impact measured
- [ ] User-facing functionality validated

## Documentation Updates
- [ ] Cross-repository documentation updated
- [ ] Integration guides updated
- [ ] API documentation synchronized
- [ ] Workflow documentation updated

## Related Issues
- Primary GitHub Issue: #[issue_number]
- Repository-specific issues:
  - [repo-1]: #[issue_number]
  - [repo-2]: #[issue_number]
  - [repo-3]: #[issue_number]

## Success Criteria
- [ ] All repository changes completed successfully
- [ ] Integration between services working correctly
- [ ] No regression in existing functionality
- [ ] Performance targets met
- [ ] All quality gates passed
- [ ] Documentation is complete and synchronized

## Risk Mitigation
- [ ] Dependencies clearly identified and managed
- [ ] Integration points thoroughly tested
- [ ] Rollback plan tested and documented
- [ ] Communication plan ensures team alignment