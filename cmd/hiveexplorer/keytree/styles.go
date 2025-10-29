package keytree

// Note: This file previously contained rendering logic (getDiffStyle, getDiffPrefix, etc.)
// that has been moved to the adapter layer as part of the component architecture refactoring.
//
// Rendering is now handled by:
// - keytree/adapter/: Business logic for converting domain Items to display properties
// - keytree/display/: Pure rendering functions with no domain knowledge
//
// See ARCHITECTURE_AUDIT.md for details on the component separation.
