-- Wyrmrest Defender (Dragonblight, quest 12372 "Defending Wyrmrest Temple") --
-- Lua port of src/server/scripts/Northrend/zone_dragonblight.cpp
-- (npc_wyrmrest_defenderAI : public VehicleAI; zone-script unit per
-- northrend_script_loader.cpp order — dragonblight: eligor
-- documented-only, defender ported here, torturer_lecraft ported).
-- Entry: creature 27629 'Wyrmrest Defender' (TDB-verified:
-- creature_template ScriptName 'npc_wyrmrest_defender', faction
-- 2010; the C++ WyrmDefenderEnum carries no NPC_ constant — the
-- ScriptName->entry binding is DB-side, ros_dark_rider precedent).
-- Eluna creature events: 14 OnHitBySpell, 23 OnReset. The C++
-- overrides no JustEngagedWith / JustDied arms — no event 1/4.
-- Ported arms (C++-exact):
-- - SpellHit (zone_dragonblight.cpp:542-562), switch on spellId:
--   SPELL_WYRMREST_DEFENDER_MOUNT 49256: Talk(WHISPER_MOUNTED 0,
--   GetCharmerOrOwner) + SetImmuneToAll(false) + SetFlag
--   (UNIT_FIELD_FLAGS, UNIT_FLAG_PLAYER_CONTROLLED). The Talk
--   bridge takes the whisper target as 2nd arg (boss_buru /
--   boss_kaelthas precedent); the target rides GetCharmer()
--   (lua_creature_events.go:721, live unit object or nil —
--   nil reads GUID 0, i.e. broadcast, the C++ empty-target
--   twin; the owner fallback only matters off the charm path,
--   where the defender is never mounted).
--   SPELL_DEFENDER_ON_LOW_HEALTH_EMOTE 52421: Talk
--   (BOSS_EMOTE_ON_LOW_HEALTH 2, GetCharmerOrOwner).
--   SPELL_RENEW 49263: the renew-recovery latch — if the
--   low-hp warning is spent and RenewRecoveryChecker <= 100,
--   set it to 20000ms and raise renewRecoveryCanCheck.
-- - Reset (zone_dragonblight.cpp:523-526): Initialize() — the
--   per-GUID latch state drops (ctor values re-latch on next
--   access, lecraft precedent).
-- Stay-outs (documented-only, no bridges):
-- - UpdateAI (zone_dragonblight.cpp:528-545): the per-tick
--   low-hp machine (hp <= 30% -> self-cast 52421, which is
--   what fires the emote Talk leg above; 20s renew-recovery
--   countdown re-arming hpWarningReady) — no per-tick
--   UpdateAI bridge. The 52421 Talk leg and the RENEW latch
--   are ported dormant: they fire / latch only if 52421 /
--   49263 is ever cast on the defender.
-- - OnGossipSelect (zone_dragonblight.cpp:564-573): menu 9568
--   option 0 -> player casts SPELL_CHARACTER_SCRIPT 49213 on
--   self + CloseGossipMenuFor — gossip-select fires
--   (gossip.go:366) but player:CastSpell is unbridged
--   (go_scripts_part1.lua header) — stays out.
-- - OnCharmed (zone_dragonblight.cpp:575-578): RemoveFlag
--   (UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) — no OnCharmed
--   bridge.
-- - VehicleAI base: mount/dismount/passenger handling is
--   engine-side; this file ports the AI arms only.
-- Dormant until seeded: 49256/52421/49263 are absent from
-- tools/spelldbc_seed/main.go, and the creature-caster ev14
-- fire is spellKnown-gated (boss_ai.go:4251) — the mount leg
-- cannot fire until the spells land in Spell.dbc (valiant
-- 63010/65147 precedent: port first, seed after via wago).

local ENTRY = 27629

local SPELL_WYRMREST_DEFENDER_MOUNT = 49256
local SPELL_DEFENDER_ON_LOW_HEALTH_EMOTE = 52421
local SPELL_RENEW = 49263

local WHISPER_MOUNTED = 0
local BOSS_EMOTE_ON_LOW_HEALTH = 2

local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h:117 (OBJECT_END + 0x35)
local UNIT_FLAG_PLAYER_CONTROLLED = 0x8 -- UnitDefines.h:127

local state = {}

local function getState(guid)
    local st = state[guid]
    if st == nil then
        st = { hpWarningReady = true, renewRecoveryCanCheck = false, renewRecoveryChecker = 0 }
        state[guid] = st
    end
    return st
end

local function onSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_WYRMREST_DEFENDER_MOUNT then
        creature:Talk(WHISPER_MOUNTED, creature:GetCharmer())
        creature:SetImmuneToAll(false)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PLAYER_CONTROLLED)
    elseif spellId == SPELL_DEFENDER_ON_LOW_HEALTH_EMOTE then
        creature:Talk(BOSS_EMOTE_ON_LOW_HEALTH, creature:GetCharmer())
    elseif spellId == SPELL_RENEW then
        local st = getState(creature:GetGUIDLow())
        if not st.hpWarningReady and st.renewRecoveryChecker <= 100 then
            st.renewRecoveryChecker = 20000
        end
        st.renewRecoveryCanCheck = true
    end
end

local function onReset(event, creature)
    state[creature:GetGUIDLow()] = nil
end

RegisterCreatureEvent(ENTRY, 14, onSpellHit)
RegisterCreatureEvent(ENTRY, 23, onReset)
