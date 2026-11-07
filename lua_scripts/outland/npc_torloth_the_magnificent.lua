-- Shadowmoon Valley: Torloth the Magnificent — Lua port of
-- src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (class npc_torloth_the_magnificent :
-- CreatureScript("npc_torloth_the_magnificent")).
-- Entry: 22076 (wowhead npc=22076/torloth-the-magnificent,
-- verified 2026-11-07; "Torloth the Magnificent <The Crimson
-- Sigil>", Shadowmoon Valley; no NPC_ constant in the C++ file).
-- Eluna creature events: 4 JustDied.
-- Ported arm (leg (a) only):
-- - JustDied (zone_shadowmoon_valley.cpp:875-893, event 4): nil
--   killer returns; TYPEID_PLAYER (4) killer ->
--   killer:GroupEventHappens(10781, creature); TYPEID_UNIT (3)
--   killer -> killer:GetOwner() -> owner
--   GroupEventHappens(10781, creature). Bridges: ev4 killer 3rd
--   arg (lua_creature_events.go:3108), GetTypeId (object.go:160),
--   GetOwner (lua_creature_events.go:627, returns the owning
--   player object or nil), GroupEventHappens (lua_player.go:4511).
-- BLOCKED leg (b) (documented-only, no bridge):
-- - JustDied :895-896: LordIllidan->AI()->EnterEvadeMode() —
--   EnterEvadeMode has NO Lua bridge (boss_ai.go:5753-5754
--   explicit). The LordIllidanGUID latch (IsSummonedBy / Reset)
--   is likewise unported; the evade leg never fires in Lua.

local ENTRY_TORLOTH = 22076

local QUEST_BATTLE_OF_THE_CRIMSON_WATCH = 10781

local TYPEID_UNIT = 3
local TYPEID_PLAYER = 4

-- C++ JustDied leg (a): quest-10781 group credit via the killer
-- (player) or the killer's owner (pet/guardian).
local function onDied(event, creature, killer)
    if not killer then
        return
    end
    if killer:GetTypeId() == TYPEID_PLAYER then
        killer:GroupEventHappens(QUEST_BATTLE_OF_THE_CRIMSON_WATCH, creature)
    elseif killer:GetTypeId() == TYPEID_UNIT then
        local owner = killer:GetOwner()
        if owner then
            owner:GroupEventHappens(QUEST_BATTLE_OF_THE_CRIMSON_WATCH, creature)
        end
    end
end

RegisterCreatureEvent(ENTRY_TORLOTH, 4, onDied)
