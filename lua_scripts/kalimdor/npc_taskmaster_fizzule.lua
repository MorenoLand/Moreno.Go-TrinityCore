-- Taskmaster Fizzule (Northern Barrens, Deep Cover rogue chain) —
-- Lua port of npc_taskmaster_fizzuleAI in
-- src/server/scripts/Kalimdor/zone_the_barrens.cpp (lines 137-238).
-- Entry (wowhead, no C++ NPC_ constant exists — the cthun/telestra
-- wowhead-only precedent, never guessed): 7233, via the wowpedia
-- Taskmaster Fizzule page's Wowhead external link
-- (wowhead.com/npc=7233).
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 14 OnSpellHit,
-- 8 OnReceiveEmote.
-- Ported arms (C++-exact):
-- - Ctor/Reset: factionNorm captured at spawn (the C++ ctor reads
--   the template faction); Reset re-initializes (IsFriend false,
--   FlareCount 0) and restores factionNorm.
-- - SpellHit (ev14): flare 10113 or folly 10137 increments
--   FlareCount; at 2+ IsFriend latches true.
-- - ReceiveEmote (ev8): salute (TEXT_EMOTE_SALUTE 78) with
--   FlareCount >= 2 and faction not already FRIENDLY runs
--   DoFriend: RemoveAllAuras + CombatStop(true) + StopMoving +
--   AttackStop + SetFaction(FRIENDLY 35) + PerformEmote
--   (EMOTE_ONESHOT_SALUTE 66).
-- Unmodeled (documented-only, no bridges):
-- - DoFriend's MoveIdle (no Lua bridge). The 120s friend revert
--   IS ported: DoFriend schedules a 120s one-shot running the
--   Reset legs (UpdateAI's EnterEvadeMode ends in Reset(),
--   CreatureAI.cpp); the evade home-walk / aura-clear legs stay
--   out, as in Reset.
-- - EngagementOver is the inline tail of the ported DoFriend arm
--   (zone_the_barrens.cpp calls it right after CombatStop): it
--   only flips _isEngaged + AtDisengage (ATTACK_PLAYER clear,
--   tapped-flag restore — no Go model, no effect for this
--   non-pet creature). The disengage core (InCombat=false,
--   threat clear, target drop) is the ported CombatStop(true)
--   above, so there is no separate missing arm.
-- - Melee is engine-driven in Go, like C++ DoMeleeAttackIfReady.

local ENTRY_FIZZULE = 7233

local SPELL_FLARE = 10113
local SPELL_FOLLY = 10137

local TEXT_EMOTE_SALUTE = 78
local EMOTE_ONESHOT_SALUTE = 66

local FACTION_FRIENDLY = 35

local state = {}
local factionNorm = {}
local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function fizzuleSpawn(event, creature)
    factionNorm[creature:GetGUID()] = creature:GetFaction()
end

local function fizzuleReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = { isFriend = false, flareCount = 0 }
    local norm = factionNorm[guid]
    if norm ~= nil then
        creature:SetFaction(norm)
    end
end

local function fizzuleSpellHit(event, creature, caster, spellId)
    local guid = creature:GetGUID()
    local st = state[guid]
    if st == nil then
        st = { isFriend = false, flareCount = 0 }
        state[guid] = st
    end
    if spellId == SPELL_FLARE or spellId == SPELL_FOLLY then
        st.flareCount = st.flareCount + 1
        if st.flareCount >= 2 then
            st.isFriend = true
        end
    end
end

local function fizzuleReceiveEmote(event, creature, player, textEmote)
    local guid = creature:GetGUID()
    local st = state[guid]
    if st == nil then
        return
    end
    if textEmote == TEXT_EMOTE_SALUTE and st.flareCount >= 2 then
        if creature:GetFaction() == FACTION_FRIENDLY then
            return
        end
        creature:RemoveAllAuras()
        creature:CombatStop(true)
        creature:StopMoving()
        creature:AttackStop()
        creature:SetFaction(FACTION_FRIENDLY)
        creature:PerformEmote(EMOTE_ONESHOT_SALUTE)
        schedule(guid, "revert", 120000, function()
            fizzuleReset(nil, creature)
        end)
    end
end

RegisterCreatureEvent(ENTRY_FIZZULE, 5, fizzuleSpawn)
RegisterCreatureEvent(ENTRY_FIZZULE, 23, fizzuleReset)
RegisterCreatureEvent(ENTRY_FIZZULE, 14, fizzuleSpellHit)
RegisterCreatureEvent(ENTRY_FIZZULE, 8, fizzuleReceiveEmote)
