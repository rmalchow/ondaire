package group

import "ondaire/internal/id"

// follow sets this node's PLAYER target (D49+ crosswise): its speakers play the
// stream of group(target). target == self ⇒ play its own group; target == Zero ⇒
// idle (same as Unfollow); a dead/unknown target ⇒ idle until that master appears.
// Any id is allowed — there is no "must be a master" rule now that every node masters
// its own group (1:1) and `following` only places the player.
func (e *Engine) follow(target id.ID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	e.setFollowing(target)
	e.log.Info("play target set", "target", target.String(), "reason", "user")
	return nil
}
