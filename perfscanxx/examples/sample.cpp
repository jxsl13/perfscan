#include <vector>
#include <string>

struct Big { double a,b,c,d,e,f,g,h; std::string name; };

int sum_lengths(const std::vector<Big>& items) {
    int total = 0;
    for (auto item : items) {          // performance-for-range-copy: copies Big each iter
        total += (int)item.name.size();
    }
    return total;
}

std::string greet(std::string who) {  // performance-unnecessary-value-param
    return "hi " + who;
}
