-- Temple of Ahn'Qiraj: Anubisath Sentinel — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/mob_anubisath_sentinel.cpp
-- (npc_anubisath_sentinelAI : public ScriptedAI via GetAQ40AI;
-- registered in AddSC_npc_anubisath_sentinel()).
-- Entry: 15264 (the buddy-grid search uses the sentinel's own entry).
-- Ported: on EVERY enter combat the sentinel picks one of the 9
-- sentinel ability buffs at random (C++ JustEngagedWith ->
-- GetOtherSentinels -> selectAbility(pickAbilityRandom); Reset()
-- re-arms gatherOthersWhenAggro, so evade -> next engage re-picks;
-- uniform 1-9 pick, matching C++ rand32()%9 on a fresh dedup set)
-- and self-casts it (GainSentinelAbility -> AddAura(id, me)).
-- Zero Talk() in C++.
-- Ported: the JustEngagedWith tail creature:DoZoneInCombat()
-- (mob_anubisath_sentinel.cpp:243, me-form on the live bridge —
-- 2026-10-23 re-triage; the old "buddy machinery" blocker was
-- inapplicable to this leg, no cross-AI/grid read is needed).
-- Documented-only: the rest of the buddy machinery
-- (AddSentinelsNear grid scan is PORTED as latchBuddies below for
-- the JustDied leg; GiveBuddyMyList/SendMyListToBuddies/
-- CallBuddiesToAttack, gatherOthersWhenAggro, cross-AI selectAbility
-- dedup, Reset buddy respawn) — no cross-AI / AttackStart bridge
-- exists in Lua. The JustDied 50% heal + ability transfer is PORTED
-- below (tick 386): GetCreaturesInGrid is bridged
-- (lua_creature_events.go:5874+), so the old "no ObjectAccessor /
-- cross-AI / GetCreatureListWithEntryInGrid / AttackStart bridge"
-- claim is stale for that leg. The Shadow Storm SDComment
-- (storm should only hit targets outside melee range) is an aura-level
-- concern, not scripted here.
local ENTRY = 15264

local BUFFS = {
    2147,  -- SPELL_MENDING_BUFF
    21737, -- SPELL_KNOCK_BUFF
    812,   -- SPELL_MANAB_BUFF
    13022, -- SPELL_REFLECTAF_BUFF
    19595, -- SPELL_REFLECTSFr_BUFF
    25777, -- SPELL_THORNS_BUFF
    2834,  -- SPELL_THUNDER_BUFF
    9347,  -- SPELL_MSTRIKE_BUFF
    2148,  -- SPELL_STORM_BUFF
}

-- guid -> {ability = spellId, buddies = {guid, ...}}: the dying
-- sentinel's own ability + its engage-time buddy list, latched so
-- sentinelDied can transfer it (mob_anubisath_sentinel.cpp:246-257).
local sentinelState = {}

local function latchBuddies(creature)
    -- C++ AddSentinelsNear (mob_anubisath_sentinel.cpp:181-189):
    -- GetCreatureListWithEntryInGrid(15264, 70.0f), AddBuddyToList
    -- caps at 3, self excluded. Lua grid order is GUID-sorted
    -- (deterministic); C++ grid-visit order is unspecified — the
    -- SET may differ when >3 sentinels are in 70yd (documented delta).
    local buddies = {}
    for _, sent in ipairs(creature:GetCreaturesInGrid(ENTRY, 70.0)) do
        if sent:GetGUID() ~= creature:GetGUID() and #buddies < 3 then
            buddies[#buddies + 1] = sent:GetGUID()
        end
    end
    return buddies
end

local function onEnterCombat(event, creature, target)
    -- C++ re-picks a random ability on EVERY engage (see header).
    local ability = BUFFS[math.random(#BUFFS)]
    creature:CastSpell(creature, ability)
    -- Latch the dying sentinel's own ability (C++ member `ability`,
    -- set in GetOtherSentinels) + its engage-time buddy list so
    -- onDied can transfer it (mob_anubisath_sentinel.cpp:246-257).
    sentinelState[creature:GetGUIDLow()] = { ability = ability, buddies = latchBuddies(creature) }
    -- mob_anubisath_sentinel.cpp:243: JustEngagedWith tail — me-form
    -- DoZoneInCombat() on the live bridge (algalon/mandokir precedent;
    -- 2026-10-23 re-triage). The old "buddy machinery" blocker is
    -- inapplicable to this leg: no ObjectAccessor / cross-AI read is
    -- needed — the bridge engages the zone directly. C++ arm order:
    -- GetOtherSentinels (unported) -> GainSentinelAbility -> this.
    creature:DoZoneInCombat()
end

local function onReset(event, creature)
    -- C++ Reset -> Initialize -> ClearBuddyList.
    sentinelState[creature:GetGUIDLow()] = nil
end

-- C++ npc_anubisath_sentinelAI::JustDied
-- (mob_anubisath_sentinel.cpp:246-257): for each of the <=3 latched
-- buddies, skip if null/dead; heal +50% max HP, transfer the dying
-- sentinel's ability (GainSentinelAbility = AddAura(id, me)).
-- The latched GUIDs are re-resolved through a fresh grid query (no
-- GUID->creature bridge exists; avoids stale stored objects).
-- Deltas: (1) buddy set is first-3 GUID-sorted vs C++ unspecified
-- grid order; (2) Lua range adds combat reaches; (3) a buddy that
-- leaves 70yd before the death is missed by the re-query — C++
-- would still find it by GUID (ObjectAccessor has no range limit).
local function sentinelDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    local st = sentinelState[guid]
    sentinelState[guid] = nil
    if st == nil or st.ability == nil then
        return
    end
    local nearby = creature:GetCreaturesInGrid(ENTRY, 70.0)
    for _, bguid in ipairs(st.buddies) do
        for _, sent in ipairs(nearby) do
            if sent:GetGUID() == bguid then
                -- C++ :252-254: skip dead buddies; heal +50% max,
                -- transfer the dying sentinel's ability.
                if not sent:IsDead() then
                    sent:SetHealth(sent:GetHealth() + sent:CountPctFromMaxHealth(50))
                    sent:AddAura(st.ability)
                end
                break
            end
        end
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 4, sentinelDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
