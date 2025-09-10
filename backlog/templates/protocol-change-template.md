# Protocol Change Template

## Protocol Change Overview
**Change Type:** [Addition/Modification/Deprecation/Breaking Change]

**Affected Protocol:** [Audio/Skills/Custom]

**Impact Level:** [Low/Medium/High/Breaking]

**Version:** [Target version for this change]

## Change Description
[Detailed description of what is changing in the protocol]

## Motivation
[Why is this change needed? What problem does it solve?]

## Affected Services
- [ ] loqa-proto (Protocol definitions)
- [ ] loqa-hub (Protocol consumer)
- [ ] loqa-relay (Protocol consumer)
- [ ] loqa-skills (Protocol consumer)
- [ ] Other: [specify]

## Backward Compatibility
- [ ] Fully backward compatible
- [ ] Backward compatible with deprecation warnings
- [ ] Breaking change (requires coordinated update)

## Protocol Changes
### New Fields/Messages
```protobuf
[Add new protocol buffer definitions here]
```

### Modified Fields/Messages
```protobuf
[Show before/after for modified definitions]
```

### Deprecated Fields/Messages
```protobuf
[List deprecated definitions with timeline]
```

## Implementation Plan
### Phase 1: Protocol Definition
- [ ] Update .proto files in loqa-proto
- [ ] Add comprehensive comments and documentation
- [ ] Generate bindings: `./generate.sh`
- [ ] Update version numbers

### Phase 2: Service Updates (in dependency order)
- [ ] Update loqa-hub implementation
- [ ] Update loqa-relay implementation
- [ ] Update loqa-skills implementation
- [ ] Update any other consumers

### Phase 3: Testing and Validation
- [ ] Unit tests for protocol serialization/deserialization
- [ ] Integration tests between services
- [ ] End-to-end testing
- [ ] Performance impact testing

## Migration Strategy
[For breaking changes, describe how to migrate existing implementations]

### Migration Timeline
- [ ] Announcement to team
- [ ] Deprecation warnings added
- [ ] Migration deadline set
- [ ] Old protocol removed

## Testing Requirements
- [ ] Protocol definition validation
- [ ] Schema compatibility testing
- [ ] Cross-service communication testing
- [ ] Backward compatibility testing (if applicable)
- [ ] Performance regression testing

## Documentation Updates
- [ ] Protocol documentation updated
- [ ] API documentation updated  
- [ ] Migration guide created (if breaking)
- [ ] Changelog updated
- [ ] Version compatibility matrix updated

## Quality Gates
- [ ] Protocol design review completed
- [ ] Breaking change approval (if applicable)
- [ ] All affected services updated
- [ ] All tests passing
- [ ] Documentation complete
- [ ] Quality check passed in all repos

## Rollout Plan
- [ ] Protocol changes deployed to development
- [ ] Service updates deployed in correct order
- [ ] Integration testing in staging
- [ ] Production rollout plan executed

## Rollback Plan
[How to rollback if issues are discovered]
- [ ] Rollback steps documented
- [ ] Rollback tested in development

## Related Issues
- GitHub Issue: #[issue_number]
- Protocol RFC: [link if applicable]
- Breaking change approval: [link]

## Acceptance Criteria
- [ ] Protocol changes are complete and tested
- [ ] All affected services are updated
- [ ] Backward compatibility maintained (if required)
- [ ] Documentation is complete and accurate
- [ ] Migration path is clear (for breaking changes)