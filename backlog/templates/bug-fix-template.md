# Bug Fix Template

## Bug Description
**Issue Summary:** [One-line description of the bug]

**Affected Component(s):** [List affected services/components]
- [ ] loqa-hub
- [ ] loqa-commander  
- [ ] loqa-relay
- [ ] loqa-proto
- [ ] loqa-skills
- [ ] www-loqalabs-com

**Severity:** [Critical/High/Medium/Low]

**Environment:** [Development/Staging/Production]

## Current Behavior
[Describe what is currently happening]

## Expected Behavior
[Describe what should happen instead]

## Steps to Reproduce
1. [First step]
2. [Second step]
3. [Third step]

## Error Messages/Logs
```
[Paste relevant error messages or logs here]
```

## Root Cause Analysis
[To be filled during investigation]

## Solution Approach
[To be filled during planning]

## Implementation Tasks
- [ ] Investigate root cause
- [ ] Implement fix
- [ ] Add/update tests
- [ ] Test fix in affected environments
- [ ] Update documentation if needed

## Testing Requirements
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed
- [ ] Regression testing completed

## Quality Gates
- [ ] Code review completed
- [ ] All tests passing
- [ ] No new linting errors
- [ ] Documentation updated
- [ ] Quality check passed: `make quality-check`

## Rollback Plan
[Describe how to rollback if the fix causes issues]

## Related Issues
- GitHub Issue: #[issue_number]
- Related PRs: [list related PRs]

## Acceptance Criteria
- [ ] Bug is resolved and no longer reproducible
- [ ] No new bugs introduced
- [ ] All affected functionality works as expected
- [ ] Performance impact is acceptable