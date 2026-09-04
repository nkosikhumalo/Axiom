public class Discount {
    public static int calculateDiscount(int amount, int tier) {
        if (amount > 1000) {
            if (tier == 2) return amount - 100;
            return amount - 50;
        }
        return amount;
    }
}
