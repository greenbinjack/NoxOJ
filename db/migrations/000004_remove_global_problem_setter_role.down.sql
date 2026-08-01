-- Note: this restores the role by name, not by its original id —
-- roles.id is a SMALLSERIAL, so a re-inserted row gets a new id.
-- Nothing depends on the specific id value, only the name (role
-- resolution is always by name — see RoleRepository.AssignRole).
INSERT INTO roles (name) VALUES ('problem_setter');
